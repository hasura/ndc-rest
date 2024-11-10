package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hasura/ndc-rest/connector/internal"
	"github.com/hasura/ndc-rest/ndc-rest-schema/configuration"
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

// GetSchema gets the connector's schema.
func (c *RESTConnector) GetSchema(ctx context.Context, configuration *configuration.Configuration, _ *State) (schema.SchemaResponseMarshaler, error) {
	return c.rawSchema, nil
}

// ApplyNDCRestSchemas applies slice of raw NDC REST schemas to the connector
func (c *RESTConnector) ApplyNDCRestSchemas(config *configuration.Configuration, schemas []configuration.NDCRestRuntimeSchema, logger *slog.Logger) error {
	ndcSchema, metadata, errs := configuration.MergeNDCRestSchemas(config, schemas)
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

	c.schema = &rest.NDCRestSchema{
		ScalarTypes: ndcSchema.ScalarTypes,
		ObjectTypes: ndcSchema.ObjectTypes,
	}
	c.metadata = metadata
	c.rawSchema = schema.NewRawSchemaResponseUnsafe(schemaBytes)

	return nil
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC REST schemas", slog.Any("errors", errors))
}

func (c *RESTConnector) parseRESTOptionsFromArguments(argumentsInfo map[string]rest.ArgumentInfo, rawArgs map[string]any) (*internal.RESTOptions, error) {
	var result internal.RESTOptions
	argInfo, ok := argumentsInfo[rest.RESTOptionsArgumentName]
	if !ok {
		return &result, nil
	}
	rawRestOptions, ok := rawArgs[rest.RESTOptionsArgumentName]
	if ok {
		if err := result.FromValue(rawRestOptions); err != nil {
			return nil, err
		}
	}
	restOptionsNamedType := schema.GetUnderlyingNamedType(argInfo.Type)
	result.Distributed = restOptionsNamedType != nil && restOptionsNamedType.Name == rest.RESTDistributedOptionsObjectName

	return &result, nil
}

func (c *RESTConnector) evalForwardedHeaders(req *internal.RetryableRequest, rawArgs map[string]any) error {
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
