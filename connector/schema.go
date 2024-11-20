package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hasura/ndc-http/connector/internal"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

// GetSchema gets the connector's schema.
func (c *HTTPConnector) GetSchema(ctx context.Context, configuration *configuration.Configuration, _ *State) (schema.SchemaResponseMarshaler, error) {
	return c.rawSchema, nil
}

// ApplyNDCHttpSchemas applies slice of raw NDC HTTP schemas to the connector
func (c *HTTPConnector) ApplyNDCHttpSchemas(config *configuration.Configuration, schemas []configuration.NDCHttpRuntimeSchema, logger *slog.Logger) error {
	ndcSchema, metadata, errs := configuration.MergeNDCHttpSchemas(config, schemas)
	if len(errs) > 0 {
		printSchemaValidationError(logger, errs)
		if ndcSchema == nil || config.Strict {
			return errBuildSchemaFailed
		}
	}

	schemaBytes, err := json.Marshal(ndcSchema.ToSchemaResponse())
	if err != nil {
		return err
	}

	c.schema = &rest.NDCHttpSchema{
		ScalarTypes: ndcSchema.ScalarTypes,
		ObjectTypes: ndcSchema.ObjectTypes,
	}
	c.metadata = metadata
	c.rawSchema = schema.NewRawSchemaResponseUnsafe(schemaBytes)

	return nil
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC HTTP schemas", slog.Any("errors", errors))
}

func (c *HTTPConnector) parseHTTPOptionsFromArguments(argumentsInfo map[string]rest.ArgumentInfo, rawArgs map[string]any) (*internal.HTTPOptions, error) {
	var result internal.HTTPOptions
	argInfo, ok := argumentsInfo[rest.HTTPOptionsArgumentName]
	if !ok {
		return &result, nil
	}
	rawHttpOptions, ok := rawArgs[rest.HTTPOptionsArgumentName]
	if ok {
		if err := result.FromValue(rawHttpOptions); err != nil {
			return nil, err
		}
	}
	httpOptionsNamedType := schema.GetUnderlyingNamedType(argInfo.Type)
	result.Distributed = httpOptionsNamedType != nil && httpOptionsNamedType.Name == rest.HTTPDistributedOptionsObjectName

	return &result, nil
}

func (c *HTTPConnector) evalForwardedHeaders(req *internal.RetryableRequest, rawArgs map[string]any) error {
	if !c.config.ForwardHeaders.Enabled || c.config.ForwardHeaders.ArgumentField == nil {
		return nil
	}
	rawHeaders, ok := rawArgs[*c.config.ForwardHeaders.ArgumentField]
	if !ok {
		return nil
	}

	var headers map[string]string
	if err := mapstructure.Decode(rawHeaders, &headers); err != nil {
		return fmt.Errorf("arguments.%s: %w", *c.config.ForwardHeaders.ArgumentField, err)
	}

	for key, value := range headers {
		if req.Headers.Get(key) != "" {
			continue
		}
		req.Headers.Set(key, value)
	}

	return nil
}
