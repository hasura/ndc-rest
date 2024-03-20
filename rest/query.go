package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	rest "github.com/hasura/ndc-rest-schema/schema"
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

	function, err := c.metadata.GetFunction(request.Collection)
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

	endpoint, headers, err := evalURLAndHeaderParameters(function.Request, function.Arguments, rawArgs)
	if err != nil {
		return nil, schema.UnprocessableContentError("failed to evaluate URL and Headers from parameters", map[string]any{
			"cause": err.Error(),
		})
	}
	// 2. create and execute request
	// 3. evaluate response selection
	function.Request.URL = endpoint

	return c.client.Send(ctx, function.Request, headers, nil, queryFields)
}

func evalURLAndHeaderParameters(request *rest.Request, argumentsSchema map[string]schema.ArgumentInfo, arguments map[string]any) (string, http.Header, error) {
	endpoint, err := url.Parse(request.URL)
	if err != nil {
		return "", nil, err
	}
	headers := http.Header{}
	for k, h := range request.Headers {
		headers.Add(k, h)
	}

	for _, param := range request.Parameters {
		argSchema, schemaOk := argumentsSchema[param.Name]
		value, ok := arguments[param.Name]

		if !schemaOk || !ok || utils.IsNil(value) {
			if param.Required {
				return "", nil, fmt.Errorf("parameter %s is required", param.Name)
			}
		} else if err := evalURLAndHeaderParameterBySchema(endpoint, &headers, &param, argSchema.Type, value); err != nil {
			return "", nil, err
		}
	}
	return endpoint.String(), headers, nil
}

func evalURLAndHeaderParameterBySchema(endpoint *url.URL, header *http.Header, param *rest.RequestParameter, argumentType schema.Type, value any) error {
	if utils.IsNil(value) {
		return nil
	}

	var valueStr string
	switch arg := argumentType.Interface().(type) {
	case *schema.NamedType:
		switch arg.Name {
		case "Boolean":
			valueStr = fmt.Sprintf("%t", value)
		case "Int", "Float", "String":
			valueStr = fmt.Sprint(value)
		default:
			b, err := json.Marshal(value)
			if err != nil {
				return err
			}
			valueStr = string(b)
		}
	case *schema.NullableType:
		return evalURLAndHeaderParameterBySchema(endpoint, header, param, arg.UnderlyingType, value)
	case *schema.ArrayType:
		if !slices.Contains([]rest.ParameterLocation{rest.InHeader, rest.InQuery}, param.In) {
			return fmt.Errorf("cannot evaluate array parameter to %s", param.In)
		}

		// TODO: evaluate array with reflection
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		valueStr = string(b)
	}

	switch param.In {
	case rest.InHeader:
		header.Set(param.Name, valueStr)
	case rest.InQuery:
		q := endpoint.Query()
		q.Add(param.Name, valueStr)
		endpoint.RawQuery = q.Encode()
	case rest.InPath:
		endpoint.Path = strings.ReplaceAll(endpoint.Path, fmt.Sprintf("{%s}", param.Name), valueStr)
	}
	return nil
}
