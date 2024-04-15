package rest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hasura/ndc-sdk-go/schema"
)

// Mutation executes a mutation.
func (c *RESTConnector) Mutation(ctx context.Context, configuration *Configuration, state *State, request *schema.MutationRequest) (*schema.MutationResponse, error) {
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

	procedure, err := c.metadata.GetProcedure(operation.Name)
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

	endpoint, headers, err := c.evalURLAndHeaderParameters(procedure.Request, procedure.Arguments, rawArgs)
	if err != nil {
		return nil, schema.BadRequestError("failed to evaluate URL and Headers from parameters", map[string]any{
			"cause": err.Error(),
		})
	}

	// 2. create and execute request
	// 3. evaluate response selection
	procedure.Request.URL = endpoint

	httpRequest, err := c.createRequest(procedure.Request, headers, rawArgs)
	if err != nil {
		return nil, err
	}

	result, err := c.client.Send(ctx, httpRequest, operation.Fields, procedure.ResultType)
	if err != nil {
		return nil, err
	}
	return schema.NewProcedureResult(result).Encode(), nil
}
