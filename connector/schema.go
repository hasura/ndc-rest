package connector

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"github.com/hasura/ndc-sdk-go/schema"
)

// GetSchema gets the connector's schema.
func (c *HTTPConnector) GetSchema(ctx context.Context, configuration *configuration.Configuration, _ *State) (schema.SchemaResponseMarshaler, error) {
	return c.rawSchema, nil
}

// ApplyNDCHttpSchemas applies slice of raw NDC HTTP schemas to the connector
func (c *HTTPConnector) ApplyNDCHttpSchemas(ctx context.Context, config *configuration.Configuration, schemas []configuration.NDCHttpRuntimeSchema, logger *slog.Logger) error {
	ndcSchema, metadata, errs := configuration.MergeNDCHttpSchemas(config, schemas)
	if len(errs) > 0 {
		printSchemaValidationError(logger, errs)
		if ndcSchema == nil || config.Strict {
			return errBuildSchemaFailed
		}
	}

	for _, meta := range metadata {
		if err := c.upstreams.Register(ctx, &meta, ndcSchema); err != nil {
			return err
		}
	}

	schemaBytes, err := json.Marshal(ndcSchema.ToSchemaResponse())
	if err != nil {
		return err
	}

	c.metadata = metadata
	c.rawSchema = schema.NewRawSchemaResponseUnsafe(schemaBytes)

	return nil
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC HTTP schemas", slog.Any("errors", errors))
}
