package rest

import (
	"context"
	"encoding/json"
	"log/slog"

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
func (c *RESTConnector) ApplyNDCRestSchemas(config *configuration.Configuration, schemas []configuration.NDCRestSchemaWithName, logger *slog.Logger) error {
	ndcSchema, metadata, errs := configuration.MergeNDCRestSchemas(schemas)
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

func parseRESTOptionsFromArguments(arguments map[string]rest.ArgumentInfo, rawRestOptions any) (*internal.RESTOptions, error) {
	var result internal.RESTOptions
	if err := result.FromValue(rawRestOptions); err != nil {
		return nil, err
	}
	argInfo, ok := arguments[rest.RESTOptionsArgumentName]
	if !ok {
		return &result, nil
	}
	restOptionsNamedType := schema.GetUnderlyingNamedType(argInfo.Type)
	result.Distributed = restOptionsNamedType != nil && restOptionsNamedType.Name == rest.RESTDistributedOptionsObjectName
	return &result, nil
}
