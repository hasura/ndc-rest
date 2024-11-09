package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/hasura/ndc-rest/connector/internal"
	"github.com/hasura/ndc-rest/ndc-rest-schema/configuration"
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
	"gopkg.in/yaml.v3"
)

// RESTConnector implements the SDK interface of NDC specification
type RESTConnector struct {
	metadata     internal.MetadataCollection
	capabilities *schema.RawCapabilitiesResponse
	rawSchema    *schema.RawSchemaResponse
	schema       *rest.NDCRestSchema
	client       *internal.HTTPClient
}

// NewRESTConnector creates a REST connector instance
func NewRESTConnector(opts ...Option) *RESTConnector {
	for _, opt := range opts {
		opt(&defaultOptions)
	}

	return &RESTConnector{
		client: internal.NewHTTPClient(defaultOptions.client),
	}
}

// ParseConfiguration validates the configuration files provided by the user, returning a validated 'Configuration',
// or throwing an error to prevents Connector startup.
func (c *RESTConnector) ParseConfiguration(ctx context.Context, configurationDir string) (*configuration.Configuration, error) {
	restCapabilities := schema.CapabilitiesResponse{
		Version: "0.1.6",
		Capabilities: schema.Capabilities{
			Query: schema.QueryCapabilities{
				Variables:    schema.LeafCapability{},
				NestedFields: schema.NestedFieldCapabilities{},
			},
			Mutation: schema.MutationCapabilities{},
		},
	}
	rawCapabilities, err := json.Marshal(restCapabilities)
	if err != nil {
		return nil, fmt.Errorf("failed to encode capabilities: %w", err)
	}
	c.capabilities = schema.NewRawCapabilitiesResponseUnsafe(rawCapabilities)

	config, err := parseConfiguration(configurationDir)
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
		schemas, errs = configuration.BuildSchemaFiles(config, configurationDir, logger)
		if len(errs) > 0 {
			printSchemaValidationError(logger, errs)
			return nil, errBuildSchemaFailed
		}
	}

	if err := c.ApplyNDCRestSchemas(config, schemas, logger); err != nil {
		return nil, errInvalidSchema
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
func (c *RESTConnector) TryInitState(ctx context.Context, configuration *configuration.Configuration, metrics *connector.TelemetryState) (*State, error) {
	c.client.SetTracer(metrics.Tracer)
	return &State{}, nil
}

// HealthCheck checks the health of the connector.
//
// For example, this function should check that the connector
// is able to reach its data source over the network.
//
// Should throw if the check fails, else resolve.
func (c *RESTConnector) HealthCheck(ctx context.Context, configuration *configuration.Configuration, state *State) error {
	return nil
}

// GetCapabilities get the connector's capabilities.
func (c *RESTConnector) GetCapabilities(configuration *configuration.Configuration) schema.CapabilitiesResponseMarshaler {
	return c.capabilities
}

func parseConfiguration(configurationDir string) (*configuration.Configuration, error) {
	var config configuration.Configuration
	jsonBytes, err := os.ReadFile(configurationDir + "/config.json")
	if err == nil {
		if err = json.Unmarshal(jsonBytes, &config); err != nil {
			return nil, err
		}
		return &config, nil
	}

	if !os.IsNotExist(err) {
		return nil, err
	}

	// try to read and parse yaml file
	yamlBytes, err := os.ReadFile(configurationDir + "/config.yaml")
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		yamlBytes, err = os.ReadFile(configurationDir + "/config.yml")
	}

	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("the config.{json,yaml,yml} file does not exist at %s", configurationDir)
		} else {
			return nil, err
		}
	}

	if err = yaml.Unmarshal(yamlBytes, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
