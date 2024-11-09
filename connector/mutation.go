package rest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hasura/ndc-rest/connector/internal"
	"github.com/hasura/ndc-rest/ndc-rest-schema/configuration"
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
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

// MutationExplain explains a mutation by creating an execution plan.
func (c *RESTConnector) MutationExplain(ctx context.Context, configuration *configuration.Configuration, state *State, request *schema.MutationRequest) (*schema.ExplainResponse, error) {
	if len(request.Operations) == 0 {
		return &schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{},
		}, nil
	}
	operation := request.Operations[0]
	switch operation.Type {
	case schema.MutationOperationProcedure:
		httpRequest, _, restOptions, err := c.explainProcedure(&operation)
		if err != nil {
			return nil, err
		}

		return serializeExplainResponse(httpRequest, restOptions)
	default:
		return nil, schema.BadRequestError(fmt.Sprintf("invalid operation type: %s", operation.Type), nil)
	}
}

func (c *RESTConnector) explainProcedure(operation *schema.MutationOperation) (*internal.RetryableRequest, *rest.OperationInfo, *internal.RESTOptions, error) {
	procedure, metadata, err := c.metadata.GetProcedure(operation.Name)
	if err != nil {
		return nil, nil, nil, err
	}

	// 1. resolve arguments, evaluate URL and query parameters
	var rawArgs map[string]any
	if err := json.Unmarshal(operation.Arguments, &rawArgs); err != nil {
		return nil, nil, nil, schema.BadRequestError("failed to decode arguments", map[string]any{
			"cause": err.Error(),
		})
	}

	// 2. build the request
	builder := internal.NewRequestBuilder(c.schema, procedure, rawArgs, metadata.Runtime)
	httpRequest, err := builder.Build()
	if err != nil {
		return nil, nil, nil, err
	}

	if err := c.evalForwardedHeaders(httpRequest, rawArgs); err != nil {
		return nil, nil, nil, schema.UnprocessableContentError("invalid forwarded headers", map[string]any{
			"cause": err.Error(),
		})
	}

	restOptions, err := c.parseRESTOptionsFromArguments(procedure.Arguments, rawArgs)
	if err != nil {
		return nil, nil, nil, schema.UnprocessableContentError("invalid rest options", map[string]any{
			"cause": err.Error(),
		})
	}

	restOptions.Settings = metadata.Settings
	return httpRequest, procedure, restOptions, nil
}

func (c *RESTConnector) execProcedure(ctx context.Context, operation *schema.MutationOperation) (schema.MutationOperationResults, error) {
	httpRequest, procedure, restOptions, err := c.explainProcedure(operation)
	if err != nil {
		return nil, err
	}

	result, err := c.client.Send(ctx, httpRequest, operation.Fields, procedure.ResultType, restOptions)
	if err != nil {
		return nil, err
	}
	return schema.NewProcedureResult(result).Encode(), nil
}
