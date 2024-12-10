package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/hasura/ndc-http/connector/internal"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
)

// HTTPConnector implements the SDK interface of NDC specification
type HTTPConnector struct {
	config       *configuration.Configuration
	metadata     internal.MetadataCollection
	capabilities *schema.RawCapabilitiesResponse
	rawSchema    *schema.RawSchemaResponse
	httpClient   *http.Client
	upstreams    *internal.UpstreamManager
}

// NewHTTPConnector creates a HTTP connector instance
func NewHTTPConnector(opts ...Option) *HTTPConnector {
	for _, opt := range opts {
		opt(&defaultOptions)
	}

	return &HTTPConnector{
		httpClient: defaultOptions.client,
	}
}

// ParseConfiguration validates the configuration files provided by the user, returning a validated 'Configuration',
// or throwing an error to prevents Connector startup.
func (c *HTTPConnector) ParseConfiguration(ctx context.Context, configurationDir string) (*configuration.Configuration, error) {
	restCapabilities := schema.CapabilitiesResponse{
		Version: "0.1.6",
		Capabilities: schema.Capabilities{
			Query: schema.QueryCapabilities{
				Variables:    schema.LeafCapability{},
				NestedFields: schema.NestedFieldCapabilities{},
				Explain:      schema.LeafCapability{},
			},
			Mutation: schema.MutationCapabilities{
				Explain: schema.LeafCapability{},
			},
		},
	}
	rawCapabilities, err := json.Marshal(restCapabilities)
	if err != nil {
		return nil, fmt.Errorf("failed to encode capabilities: %w", err)
	}
	c.capabilities = schema.NewRawCapabilitiesResponseUnsafe(rawCapabilities)

	config, err := configuration.ReadConfigurationFile(configurationDir)
	if err != nil {
		return nil, err
	}

	logger := connector.GetLogger(ctx)
	schemas, err := configuration.ReadSchemaOutputFile(configurationDir, config.Output, logger)
	if err != nil {
		return nil, err
	}

	var errs map[string][]string
	if schemas == nil {
		logger.Debug(fmt.Sprintf("output file at %s does not exist. Parsing files...", filepath.Join(configurationDir, config.Output)))
		schemas, errs = configuration.BuildSchemaFromConfig(config, configurationDir, logger)
		if len(errs) > 0 {
			printSchemaValidationError(logger, errs)

			return nil, errBuildSchemaFailed
		}
	}

	c.config = config
	c.upstreams = internal.NewUpstreamManager(c.httpClient, config)
	if err := c.ApplyNDCHttpSchemas(ctx, config, schemas, logger); err != nil {
		return nil, fmt.Errorf("failed to validate NDC HTTP schema: %w", err)
	}

	return config, nil
}

// TryInitState initializes the connector's in-memory state.
//
// For example, any connection pools, prepared queries,
// or other managed resources would be allocated here.
//
// In addition, this function should register any
// connector-specific metrics with the metrics registry.
func (c *HTTPConnector) TryInitState(ctx context.Context, configuration *configuration.Configuration, metrics *connector.TelemetryState) (*State, error) {
	return &State{
		Tracer: metrics.Tracer,
	}, nil
}

// HealthCheck checks the health of the connector.
//
// For example, this function should check that the connector
// is able to reach its data source over the network.
//
// Should throw if the check fails, else resolve.
func (c *HTTPConnector) HealthCheck(ctx context.Context, configuration *configuration.Configuration, state *State) error {
	return nil
}

// GetCapabilities get the connector's capabilities.
func (c *HTTPConnector) GetCapabilities(configuration *configuration.Configuration) schema.CapabilitiesResponseMarshaler {
	return c.capabilities
}
