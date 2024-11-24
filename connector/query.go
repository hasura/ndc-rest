package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/hasura/ndc-http/connector/internal"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/errgroup"
)

// Query executes a query.
func (c *HTTPConnector) Query(ctx context.Context, configuration *configuration.Configuration, state *State, request *schema.QueryRequest) (schema.QueryResponse, error) {
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
func (c *HTTPConnector) QueryExplain(ctx context.Context, configuration *configuration.Configuration, state *State, request *schema.QueryRequest) (*schema.ExplainResponse, error) {
	requestVars := request.Variables
	if len(requestVars) == 0 {
		requestVars = []schema.QueryRequestVariablesElem{make(schema.QueryRequestVariablesElem)}
	}

	httpRequest, _, _, httpOptions, err := c.explainQuery(request, requestVars[0])
	if err != nil {
		return nil, err
	}

	return serializeExplainResponse(httpRequest, httpOptions)
}

func (c *HTTPConnector) explainQuery(request *schema.QueryRequest, variables map[string]any) (*internal.RetryableRequest, *rest.OperationInfo, *configuration.NDCHttpRuntimeSchema, *internal.HTTPOptions, error) {
	function, metadata, err := c.metadata.GetFunction(request.Collection)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// 1. resolve arguments, evaluate URL and query parameters
	rawArgs, err := utils.ResolveArgumentVariables(request.Arguments, variables)
	if err != nil {
		return nil, nil, nil, nil, schema.UnprocessableContentError("failed to resolve argument variables", map[string]any{
			"cause": err.Error(),
		})
	}

	// 2. build the request
	req, err := internal.NewRequestBuilder(c.schema, function, rawArgs, metadata.Runtime).Build()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if err := c.evalForwardedHeaders(req, rawArgs); err != nil {
		return nil, nil, nil, nil, schema.UnprocessableContentError("invalid forwarded headers", map[string]any{
			"cause": err.Error(),
		})
	}

	httpOptions, err := c.parseHTTPOptionsFromArguments(function.Arguments, rawArgs)
	if err != nil {
		return nil, nil, nil, nil, schema.UnprocessableContentError("invalid http options", map[string]any{
			"cause": err.Error(),
		})
	}

	httpOptions.Settings = metadata.Settings

	return req, function, &metadata, httpOptions, err
}

func (c *HTTPConnector) execQuerySync(ctx context.Context, state *State, request *schema.QueryRequest, valueField schema.NestedField, requestVars []schema.QueryRequestVariablesElem) ([]schema.RowSet, error) {
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

func (c *HTTPConnector) execQueryAsync(ctx context.Context, state *State, request *schema.QueryRequest, valueField schema.NestedField, requestVars []schema.QueryRequestVariablesElem) ([]schema.RowSet, error) {
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

func (c *HTTPConnector) execQuery(ctx context.Context, state *State, request *schema.QueryRequest, queryFields schema.NestedField, variables map[string]any, index int) (any, error) {
	ctx, span := state.Tracer.Start(ctx, fmt.Sprintf("Execute Query %d", index))
	defer span.End()

	httpRequest, function, metadata, httpOptions, err := c.explainQuery(request, variables)
	if err != nil {
		span.SetStatus(codes.Error, "failed to explain query")
		span.RecordError(err)

		return nil, err
	}

	httpOptions.Concurrency = c.config.Concurrency.HTTP
	client := internal.NewHTTPClient(c.client, metadata.NDCHttpSchema, c.config.ForwardHeaders, state.Tracer)
	result, _, err := client.Send(ctx, httpRequest, queryFields, function.ResultType, httpOptions)
	if err != nil {
		span.SetStatus(codes.Error, "failed to execute the http request")
		span.RecordError(err)

		return nil, err
	}

	return result, nil
}

func serializeExplainResponse(httpRequest *internal.RetryableRequest, httpOptions *internal.HTTPOptions) (*schema.ExplainResponse, error) {
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

	httpOptions.Distributed = false
	httpOptions.Explain = true
	requests, err := internal.BuildDistributedRequestsWithOptions(httpRequest, httpOptions)
	if err != nil {
		return nil, err
	}
	explainResp.Details["url"] = requests[0].URL.String()

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
