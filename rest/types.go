package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/hasura/ndc-rest-schema/command"
	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

const (
	defaultTimeoutSeconds uint = 30
	defaultRetryDelays    uint = 1000

	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
)

var defaultRetryHTTPStatus = []int64{429, 500, 502, 503}

// Configuration contains required settings for the connector.
type Configuration struct {
	Files []command.ConvertConfig `json:"files" yaml:"files"`
}

// State is the global state which is shared for every connector request.
type State struct {
	Schema *rest.NDCRestSchema
}

type options struct {
	client Doer
}

var defaultOptions options = options{
	client: &http.Client{
		Transport: http.DefaultTransport,
	},
}

// Option is an interface to set custom REST connector options
type Option (func(*options))

// WithClient sets the custom HTTP client that satisfy the Doer interface
func WithClient(client Doer) Option {
	return func(opts *options) {
		opts.client = client
	}
}

type ndcRestSchemaWithName struct {
	name   string
	schema *rest.NDCRestSchema
}

// RESTExecutionStrategy represents the execution strategy to remote servers
type RESTExecutionStrategy string

const (
	ExecuteSingleServer    RESTExecutionStrategy = "single"
	ExecuteAllServersSync  RESTExecutionStrategy = "all-sync"
	ExecuteAllServersAsync RESTExecutionStrategy = "all-async"
)

var restExecutionStrategy_enums = []RESTExecutionStrategy{
	ExecuteSingleServer,
	ExecuteAllServersSync,
	ExecuteAllServersAsync,
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *RESTExecutionStrategy) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseRESTExecutionStrategy(rawResult)
	if err != nil {
		return err
	}

	*j = result
	return nil
}

// IsEmpty checks if the style enum is valid
func (j RESTExecutionStrategy) IsValid() bool {
	return slices.Contains(restExecutionStrategy_enums, j)
}

func (j RESTExecutionStrategy) ScalarName() string {
	return "RestExecutionStrategy"
}

// ScalarType returns the scalar type definition
func (j RESTExecutionStrategy) ScalarType() *schema.ScalarType {
	oneOf := make([]string, len(restExecutionStrategy_enums))
	for i, enum := range restExecutionStrategy_enums {
		oneOf[i] = string(enum)
	}
	result := schema.NewScalarType()
	result.Representation = schema.NewTypeRepresentationEnum(oneOf).Encode()
	return result
}

// ParseRESTExecutionStrategy parses RESTExecutionStrategy from string
func ParseRESTExecutionStrategy(input string) (RESTExecutionStrategy, error) {
	result := RESTExecutionStrategy(input)
	if !result.IsValid() {
		return result, fmt.Errorf("invalid RestExecutionStrategy. Expected %+v, got <%s>", restExecutionStrategy_enums, input)
	}
	return result, nil
}

const (
	RESTOptionsArgumentName string = "restOptions"
	RESTOptionsObjectName   string = "RestOptions"
	RESTServerIDScalarName  string = "RestServerId"
)

// RESTOptions represent execution options for REST requests
type RESTOptions struct {
	ServerIDs []string              `json:"serverIds" yaml:"serverIds" mapstructure:"serverIds"`
	Strategy  RESTExecutionStrategy `json:"strategy,omitempty" yaml:"strategy,omitempty" mapstructure:"strategy"`
}

// ObjectName returns the NDC object name
func (ro RESTOptions) ObjectName() string {
	return RESTOptionsObjectName
}

// ObjectType returns the NDC object type schema
func (ro RESTOptions) ObjectType() *schema.ObjectType {
	return &schema.ObjectType{
		Description: utils.ToPtr("execution options for REST requests"),
		Fields: schema.ObjectTypeFields{
			"serverIds": schema.ObjectField{
				Description: utils.ToPtr("specify remote servers to receive the request"),
				Type:        schema.NewNullableArrayType(schema.NewNamedType(RESTServerIDScalarName)).Encode(),
			},
			"strategy": schema.ObjectField{
				Description: utils.ToPtr("specify the execution strategy to remote servers"),
				Type:        schema.NewNullableNamedType(RESTExecutionStrategy("").ScalarName()).Encode(),
			},
		},
	}
}
