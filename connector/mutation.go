package rest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hasura/ndc-rest/connector/internal"
	"github.com/hasura/ndc-rest/ndc-rest-schema/configuration"
	"github.com/hasura/ndc-sdk-go/schema"
)

// Mutation executes a mutation.
func (c *RESTConnector) Mutation(ctx context.Context, configuration *configuration.Configuration, state *State, request *schema.MutationRequest) (*schema.MutationResponse, error) {
	operationResults := make([]schema.MutationOperationResults, len(request.Operations))

	for i, operation := range request.Operations {
		switch operation.Type {
		case schema.MutationOperationProcedure:
			result, err := c.execProcedure(ctx, &operation)
			if err != nil {
				return nil, err
			}
			operationResults[i] = result
		default:
			return nil, schema.BadRequestError(fmt.Sprintf("invalid operation type: %s", operation.Type), nil)
		}
	}

	return &schema.MutationResponse{
		OperationResults: operationResults,
	}, nil
}

func (c *RESTConnector) execProcedure(ctx context.Context, operation *schema.MutationOperation) (schema.MutationOperationResults, error) {
	procedure, settings, err := c.metadata.GetProcedure(operation.Name)
	if err != nil {
		return nil, err
	}

	// 1. resolve arguments, evaluate URL and query parameters
	var rawArgs map[string]any
	if err := json.Unmarshal(operation.Arguments, &rawArgs); err != nil {
		return nil, schema.BadRequestError("failed to decode arguments", map[string]any{
			"cause": err.Error(),
		})
	}

	// 2. build the request
	builder := internal.NewRequestBuilder(c.schema, procedure, rawArgs)
	httpRequest, err := builder.Build()
	if err != nil {
		return nil, err
	}

	restOptions, err := parseRESTOptionsFromArguments(procedure.Arguments, rawArgs[internal.RESTOptionsArgumentName])
	if err != nil {
		return nil, schema.UnprocessableContentError("invalid rest options", map[string]any{
			"cause": err.Error(),
		})
	}

	restOptions.Settings = settings

	// 3. execute the request and evaluate response selection
	result, err := c.client.Send(ctx, httpRequest, operation.Fields, procedure.ResultType, restOptions)
	if err != nil {
		return nil, err
	}
	return schema.NewProcedureResult(result).Encode(), nil
}
