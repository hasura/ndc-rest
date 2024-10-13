package rest

import (
	"context"

	"github.com/hasura/ndc-rest/rest/internal"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// Query executes a query.
func (c *RESTConnector) Query(ctx context.Context, configuration *Configuration, state *State, request *schema.QueryRequest) (schema.QueryResponse, error) {
	valueField, err := utils.EvalFunctionSelectionFieldValue(request)
	if err != nil {
		return nil, schema.UnprocessableContentError(err.Error(), nil)
	}
	requestVars := request.Variables
	if len(requestVars) == 0 {
		requestVars = []schema.QueryRequestVariablesElem{make(schema.QueryRequestVariablesElem)}
	}

	rowSets := make([]schema.RowSet, len(requestVars))
	for i, requestVar := range requestVars {
		result, err := c.execQuery(ctx, request, valueField, requestVar)
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

func (c *RESTConnector) execQuery(ctx context.Context, request *schema.QueryRequest, queryFields schema.NestedField, variables map[string]any) (any, error) {
	function, settings, err := c.metadata.GetFunction(request.Collection)
	if err != nil {
		return nil, err
	}

	// 1. resolve arguments, evaluate URL and query parameters
	rawArgs, err := utils.ResolveArgumentVariables(request.Arguments, variables)
	if err != nil {
		return nil, schema.UnprocessableContentError("failed to resolve argument variables", map[string]any{
			"cause": err.Error(),
		})
	}

	endpoint, headers, err := c.evalURLAndHeaderParameters(function.Request, function.Arguments, rawArgs)
	if err != nil {
		return nil, schema.UnprocessableContentError("failed to evaluate URL and Headers from parameters", map[string]any{
			"cause": err.Error(),
		})
	}

	restOptions, err := parseRESTOptionsFromArguments(function.Arguments, rawArgs[internal.RESTOptionsArgumentName])
	if err != nil {
		return nil, schema.UnprocessableContentError("invalid rest options", map[string]any{
			"cause": err.Error(),
		})
	}

	// 2. create and execute request
	// 3. evaluate response selection
	restOptions.Settings = settings
	httpRequest, err := c.createRequest(function, endpoint, headers, nil)
	if err != nil {
		return nil, err
	}

	return c.client.Send(ctx, httpRequest, queryFields, function.ResultType, restOptions)
}
