package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hasura/ndc-http/connector/internal"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/errgroup"
)

// Mutation executes a mutation.
func (c *HTTPConnector) Mutation(ctx context.Context, configuration *configuration.Configuration, state *State, request *schema.MutationRequest) (*schema.MutationResponse, error) {
	if len(request.Operations) == 1 || c.config.Concurrency.Mutation <= 1 {
		return c.execMutationSync(ctx, state, request)
	}

	return c.execMutationAsync(ctx, state, request)
}

// MutationExplain explains a mutation by creating an execution plan.
func (c *HTTPConnector) MutationExplain(ctx context.Context, configuration *configuration.Configuration, state *State, request *schema.MutationRequest) (*schema.ExplainResponse, error) {
	if len(request.Operations) == 0 {
		return &schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{},
		}, nil
	}
	operation := request.Operations[0]
	switch operation.Type {
	case schema.MutationOperationProcedure:
		httpRequest, _, httpOptions, err := c.explainProcedure(&operation)
		if err != nil {
			return nil, err
		}

		return serializeExplainResponse(httpRequest, httpOptions)
	default:
		return nil, schema.BadRequestError(fmt.Sprintf("invalid operation type: %s", operation.Type), nil)
	}
}

func (c *HTTPConnector) explainProcedure(operation *schema.MutationOperation) (*internal.RetryableRequest, *rest.OperationInfo, *internal.HTTPOptions, error) {
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

	httpOptions, err := c.parseHTTPOptionsFromArguments(procedure.Arguments, rawArgs)
	if err != nil {
		return nil, nil, nil, schema.UnprocessableContentError("invalid http options", map[string]any{
			"cause": err.Error(),
		})
	}

	httpOptions.Settings = metadata.Settings

	return httpRequest, procedure, httpOptions, nil
}

func (c *HTTPConnector) execMutationSync(ctx context.Context, state *State, request *schema.MutationRequest) (*schema.MutationResponse, error) {
	operationResults := make([]schema.MutationOperationResults, len(request.Operations))
	for i, operation := range request.Operations {
		result, err := c.execMutationOperation(ctx, state, operation, i)
		if err != nil {
			return nil, err
		}
		operationResults[i] = result
	}

	return &schema.MutationResponse{
		OperationResults: operationResults,
	}, nil
}

func (c *HTTPConnector) execMutationAsync(ctx context.Context, state *State, request *schema.MutationRequest) (*schema.MutationResponse, error) {
	operationResults := make([]schema.MutationOperationResults, len(request.Operations))

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(int(c.config.Concurrency.Mutation))

	for i, operation := range request.Operations {
		func(index int, op schema.MutationOperation) {
			eg.Go(func() error {
				result, err := c.execMutationOperation(ctx, state, op, index)
				if err != nil {
					return err
				}
				operationResults[index] = result

				return nil
			})
		}(i, operation)
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return &schema.MutationResponse{
		OperationResults: operationResults,
	}, nil
}

func (c *HTTPConnector) execMutationOperation(parentCtx context.Context, state *State, operation schema.MutationOperation, index int) (schema.MutationOperationResults, error) {
	ctx, span := state.Tracer.Start(parentCtx, fmt.Sprintf("Execute Operation %d", index))
	defer span.End()

	httpRequest, procedure, httpOptions, err := c.explainProcedure(&operation)
	if err != nil {
		span.SetStatus(codes.Error, "failed to explain mutation")
		span.RecordError(err)

		return nil, err
	}

	httpOptions.Concurrency = c.config.Concurrency.HTTP
	result, headers, err := c.client.Send(ctx, httpRequest, operation.Fields, procedure.ResultType, httpOptions)
	if err != nil {
		span.SetStatus(codes.Error, "failed to execute mutation")
		span.RecordError(err)

		return nil, err
	}

	return schema.NewProcedureResult(c.createHeaderForwardingResponse(result, headers)).Encode(), nil
}
