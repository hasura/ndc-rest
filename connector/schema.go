package connector

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hasura/ndc-http/connector/internal"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"github.com/hasura/ndc-sdk-go/schema"
)

// GetSchema gets the connector's schema.
func (c *HTTPConnector) GetSchema(ctx context.Context, configuration *configuration.Configuration, _ *State) (schema.SchemaResponseMarshaler, error) {
	return c.rawSchema, nil
}

// ApplyNDCHttpSchemas applies slice of raw NDC HTTP schemas to the connector
func (c *HTTPConnector) ApplyNDCHttpSchemas(ctx context.Context, config *configuration.Configuration, schemas []configuration.NDCHttpRuntimeSchema, logger *slog.Logger) error {
	httpSchema, metadata, errs := configuration.MergeNDCHttpSchemas(config, schemas)
	if len(errs) > 0 {
		printSchemaValidationError(logger, errs)
		if httpSchema == nil || config.Strict {
			return errBuildSchemaFailed
		}
	}

	for _, meta := range metadata {
		if err := c.upstreams.Register(ctx, &meta, httpSchema); err != nil {
			return err
		}
	}

	ndcSchema, procSendHttp := internal.ApplyDefaultConnectorSchema(httpSchema.ToSchemaResponse(), config.ForwardHeaders)
	schemaBytes, err := json.Marshal(ndcSchema)
	if err != nil {
		return err
	}

	c.metadata = metadata
	c.rawSchema = schema.NewRawSchemaResponseUnsafe(schemaBytes)
	c.procSendHttpRequest = procSendHttp

	return nil
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC HTTP schemas", slog.Any("errors", errors))
}
