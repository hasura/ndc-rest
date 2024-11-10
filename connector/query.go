package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/hasura/ndc-rest/connector/internal"
	"github.com/hasura/ndc-rest/ndc-rest-schema/configuration"
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/errgroup"
)

// Query executes a query.
func (c *RESTConnector) Query(ctx context.Context, configuration *configuration.Configuration, state *State, request *schema.QueryRequest) (schema.QueryResponse, error) {
	valueField, err := utils.EvalFunctionSelectionFieldValue(request)
	if err != nil {
		return nil, schema.UnprocessableContentError(err.Error(), nil)
	}
	requestVars := request.Variables
	if len(requestVars) == 0 {
		requestVars = []schema.QueryRequestVariablesElem{make(schema.QueryRequestVariablesElem)}
	}

	if len(requestVars) == 1 || c.config.Concurrency.Query <= 1 {
		return c.execQuerySync(ctx, state, request, valueField, requestVars)
	}

	return c.execQueryAsync(ctx, state, request, valueField, requestVars)
}

// QueryExplain explains a query by creating an execution plan.
func (c *RESTConnector) QueryExplain(ctx context.Context, configuration *configuration.Configuration, state *State, request *schema.QueryRequest) (*schema.ExplainResponse, error) {
	requestVars := request.Variables
	if len(requestVars) == 0 {
		requestVars = []schema.QueryRequestVariablesElem{make(schema.QueryRequestVariablesElem)}
	}

	httpRequest, _, restOptions, err := c.explainQuery(request, requestVars[0])
	if err != nil {
		return nil, err
	}

	return serializeExplainResponse(httpRequest, restOptions)
}

func (c *RESTConnector) explainQuery(request *schema.QueryRequest, variables map[string]any) (*internal.RetryableRequest, *rest.OperationInfo, *internal.RESTOptions, error) {
	function, metadata, err := c.metadata.GetFunction(request.Collection)
	if err != nil {
		return nil, nil, nil, err
	}

	// 1. resolve arguments, evaluate URL and query parameters
	rawArgs, err := utils.ResolveArgumentVariables(request.Arguments, variables)
	if err != nil {
		return nil, nil, nil, schema.UnprocessableContentError("failed to resolve argument variables", map[string]any{
			"cause": err.Error(),
		})
	}

	// 2. build the request
	req, err := internal.NewRequestBuilder(c.schema, function, rawArgs, metadata.Runtime).Build()
	if err != nil {
		return nil, nil, nil, err
	}

	if err := c.evalForwardedHeaders(req, rawArgs); err != nil {
		return nil, nil, nil, schema.UnprocessableContentError("invalid forwarded headers", map[string]any{
			"cause": err.Error(),
		})
	}

	restOptions, err := c.parseRESTOptionsFromArguments(function.Arguments, rawArgs)
	if err != nil {
		return nil, nil, nil, schema.UnprocessableContentError("invalid rest options", map[string]any{
			"cause": err.Error(),
		})
	}

	restOptions.Settings = metadata.Settings

	return req, function, restOptions, err
}

func (c *RESTConnector) execQuerySync(ctx context.Context, state *State, request *schema.QueryRequest, valueField schema.NestedField, requestVars []schema.QueryRequestVariablesElem) ([]schema.RowSet, error) {
	rowSets := make([]schema.RowSet, len(requestVars))

	for i, requestVar := range requestVars {
		result, err := c.execQuery(ctx, state, request, valueField, requestVar, i)
		if err != nil {
			return nil, err
		}
		rowSets[i] = schema.RowSet{
			Aggregates: schema.RowSetAggregates{},
			Rows: []map[string]any{
				{
					"__value": result,
				},
			},
		}
	}

	return rowSets, nil
}

func (c *RESTConnector) execQueryAsync(ctx context.Context, state *State, request *schema.QueryRequest, valueField schema.NestedField, requestVars []schema.QueryRequestVariablesElem) ([]schema.RowSet, error) {
	rowSets := make([]schema.RowSet, len(requestVars))

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(int(c.config.Concurrency.Query))

	for i, requestVar := range requestVars {
		func(index int, vars schema.QueryRequestVariablesElem) {
			eg.Go(func() error {
				result, err := c.execQuery(ctx, state, request, valueField, requestVar, i)
				if err != nil {
					return err
				}
				rowSets[index] = schema.RowSet{
					Aggregates: schema.RowSetAggregates{},
					Rows: []map[string]any{
						{
							"__value": result,
						},
					},
				}

				return nil
			})
		}(i, requestVar)
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return rowSets, nil
}

func (c *RESTConnector) execQuery(ctx context.Context, state *State, request *schema.QueryRequest, queryFields schema.NestedField, variables map[string]any, index int) (any, error) {
	ctx, span := state.Tracer.Start(ctx, fmt.Sprintf("Execute Query %d", index))
	defer span.End()

	httpRequest, function, restOptions, err := c.explainQuery(request, variables)
	if err != nil {
		span.SetStatus(codes.Error, "failed to explain query")
		span.RecordError(err)
		return nil, err
	}

	restOptions.Concurrency = c.config.Concurrency.REST
	result, headers, err := c.client.Send(ctx, httpRequest, queryFields, function.ResultType, restOptions)
	if err != nil {
		span.SetStatus(codes.Error, "failed to execute the http request")
		span.RecordError(err)

		return nil, err
	}

	return c.createHeaderForwardingResponse(result, headers), nil
}

func serializeExplainResponse(httpRequest *internal.RetryableRequest, restOptions *internal.RESTOptions) (*schema.ExplainResponse, error) {
	explainResp := &schema.ExplainResponse{
		Details: schema.ExplainResponseDetails{},
	}
	if httpRequest.Body != nil {
		bodyBytes, err := io.ReadAll(httpRequest.Body)
		if err != nil {
			return nil, schema.InternalServerError("failed to read request body", map[string]any{
				"cause": err.Error(),
			})
		}
		httpRequest.Body = nil
		explainResp.Details["body"] = string(bodyBytes)
	}

	restOptions.Distributed = false
	restOptions.Explain = true
	requests, err := internal.BuildDistributedRequestsWithOptions(httpRequest, restOptions)
	if err != nil {
		return nil, err
	}
	explainResp.Details["url"] = requests[0].URL

	if httpRequest.Body != nil {
		bodyBytes, err := io.ReadAll(httpRequest.Body)
		if err != nil {
			return nil, schema.InternalServerError("failed to read request body", map[string]any{
				"cause": err.Error(),
			})
		}
		httpRequest.Body = nil
		explainResp.Details["body"] = string(bodyBytes)
	}
	rawHeaders, err := json.Marshal(requests[0].Headers)
	if err != nil {
		return nil, schema.InternalServerError("failed to encode headers", map[string]any{
			"cause": err.Error(),
		})
	}
	explainResp.Details["headers"] = string(rawHeaders)

	return explainResp, nil
}
