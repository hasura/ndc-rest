package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/hasura/ndc-http/connector/internal"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
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

	requests, err := c.explainQuery(request, requestVars[0])
	if err != nil {
		return nil, err
	}

	return c.serializeExplainResponse(ctx, requests)
}

func (c *HTTPConnector) explainQuery(request *schema.QueryRequest, variables map[string]any) (*internal.RequestBuilderResults, error) {
	function, metadata, err := c.metadata.GetFunction(request.Collection)
	if err != nil {
		return nil, err
	}

	rawArgs, err := utils.ResolveArgumentVariables(request.Arguments, variables)
	if err != nil {
		return nil, schema.UnprocessableContentError("failed to resolve argument variables", map[string]any{
			"cause": err.Error(),
		})
	}

	return c.upstreams.BuildRequests(metadata, request.Collection, function, rawArgs)
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

	requests, err := c.explainQuery(request, variables)
	if err != nil {
		span.SetStatus(codes.Error, "failed to explain query")
		span.RecordError(err)

		return nil, err
	}

	client := internal.NewHTTPClient(c.upstreams, requests, c.config.ForwardHeaders)
	result, _, err := client.Send(ctx, queryFields)
	if err != nil {
		span.SetStatus(codes.Error, "failed to execute the http request")
		span.RecordError(err)

		return nil, err
	}

	return result, nil
}

func (c *HTTPConnector) serializeExplainResponse(ctx context.Context, requests *internal.RequestBuilderResults) (*schema.ExplainResponse, error) {
	explainResp := &schema.ExplainResponse{
		Details: schema.ExplainResponseDetails{},
	}
	httpRequest := requests.Requests[0]
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

	req, cancel, err := httpRequest.CreateRequest(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()

	c.upstreams.InjectMockRequestSettings(req, requests.Schema.Name, httpRequest.RawRequest.Security)

	explainResp.Details["url"] = req.URL.String()
	rawHeaders, err := json.Marshal(req.Header)
	if err != nil {
		return nil, schema.InternalServerError("failed to encode headers", map[string]any{
			"cause": err.Error(),
		})
	}
	explainResp.Details["headers"] = string(rawHeaders)

	return explainResp, nil
}
