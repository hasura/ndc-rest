package internal

import (
	"encoding/json"
	"errors"
	"fmt"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

const (
	contentTypeHeader          = "Content-Type"
	defaultTimeoutSeconds uint = 30
	defaultRetryDelays    uint = 1000
)

var (
	errArgumentRequired        = errors.New("argument is required")
	errRequestBodyRequired     = errors.New("request body is required")
	errRequestBodyTypeRequired = errors.New("failed to decode request body, empty body type")
)

var defaultRetryHTTPStatus = []int64{429, 500, 502, 503}

const (
	RESTOptionsArgumentName          string = "restOptions"
	RESTSingleOptionsObjectName      string = "RestSingleOptions"
	RESTDistributedOptionsObjectName string = "RestDistributedOptions"
	RESTServerIDScalarName           string = "RestServerId"
	DistributedErrorObjectName       string = "DistributedError"
)

// SingleObjectType represents the object type of REST execution options for single server
var SingleObjectType rest.ObjectType = rest.ObjectType{
	Description: utils.ToPtr("Execution options for REST requests to a single server"),
	Fields: map[string]rest.ObjectField{
		"servers": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr("Specify remote servers to receive the request. If there are many server IDs the server is selected randomly"),
				Type:        schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(RESTServerIDScalarName))).Encode(),
			},
		},
	},
}

// DistributedObjectType represents the object type of REST execution options for distributed servers
var DistributedObjectType rest.ObjectType = rest.ObjectType{
	Description: utils.ToPtr("Distributed execution options for REST requests to multiple servers"),
	Fields: map[string]rest.ObjectField{
		"servers": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr("Specify remote servers to receive the request"),
				Type:        schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(RESTServerIDScalarName))).Encode(),
			},
		},
		"parallel": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr("Execute requests to remote servers in parallel"),
				Type:        schema.NewNullableNamedType(string(rest.ScalarBoolean)).Encode(),
			},
		},
	},
}

// RESTOptions represent execution options for REST requests
type RESTOptions struct {
	Servers  []string `json:"servers"  yaml:"serverIds"`
	Parallel bool     `json:"parallel" yaml:"parallel"`

	Distributed bool                  `json:"-" yaml:"-"`
	Settings    *rest.NDCRestSettings `json:"-" yaml:"-"`
}

// FromValue parses rest execution options from any value
func (ro *RESTOptions) FromValue(value any) error {
	if utils.IsNil(value) {
		return nil
	}
	valueMap, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid rest options; expected object, got %v", value)
	}
	rawServerIds, err := utils.GetNullableStringSlice(valueMap, "servers")
	if err != nil {
		return fmt.Errorf("invalid rest options; %w", err)
	}
	if rawServerIds != nil {
		ro.Servers = *rawServerIds
	}

	parallel, err := utils.GetNullableBoolean(valueMap, "parallel")
	if err != nil {
		return fmt.Errorf("invalid parallel in rest options: %w", err)
	}
	ro.Parallel = parallel != nil && *parallel

	return nil
}

// DistributedError represents the error response of the remote request
type DistributedError struct {
	schema.ConnectorError

	// Identity of the remote server
	Server string `json:"server" yaml:"server"`
}

// Error implements the Error interface
func (de DistributedError) Error() string {
	if de.Message != "" {
		return fmt.Sprintf("%s: %s", de.Server, de.Message)
	}
	bs, err := json.Marshal(de.Details)
	if err != nil {
		bs = []byte("")
	}
	return fmt.Sprintf("%s: %s", de.Server, string(bs))
}

// DistributedResult contains the success response of remote requests with a server identity
type DistributedResult[T any] struct {
	Server string `json:"server" yaml:"server"`
	Data   T      `json:"data"   yaml:"data"`
}

// DistributedResponse represents the response object of distributed operations
type DistributedResponse[T any] struct {
	Results []DistributedResult[T] `json:"results" yaml:"results"`
	Errors  []DistributedError     `json:"errors"  yaml:"errors"`
}

// NewDistributedResponse creates an empty DistributedResponse instance
func NewDistributedResponse[T any]() *DistributedResponse[T] {
	return &DistributedResponse[T]{
		Results: []DistributedResult[T]{},
		Errors:  []DistributedError{},
	}
}
