package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

const (
	acceptHeader               = "Accept"
	acceptEncodingHeader       = "Accept-Encoding"
	defaultTimeoutSeconds uint = 30
	defaultRetryDelays    uint = 1000
)

var (
	errRequestBodyRequired = errors.New("request body is required")
)

var defaultRetryHTTPStatus = []int{429, 500, 502, 503}
var sensitiveHeaderRegex = regexp.MustCompile(`auth|key|secret|token`)
var urlAndHeaderLocations = []rest.ParameterLocation{rest.InPath, rest.InQuery, rest.InHeader}

// HTTPOptions represent execution options for HTTP requests
type HTTPOptions struct {
	Servers  []string `json:"serverIds" yaml:"serverIds"`
	Parallel bool     `json:"parallel"  yaml:"parallel"`

	Distributed bool `json:"-" yaml:"-"`
	Concurrency uint `json:"-" yaml:"-"`
}

// FromValue parses http execution options from any value
func (ro *HTTPOptions) FromValue(value any) error {
	if utils.IsNil(value) {
		return nil
	}
	valueMap, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid http options; expected object, got %v", value)
	}
	rawServerIds, err := utils.GetNullableStringSlice(valueMap, "servers")
	if err != nil {
		return fmt.Errorf("invalid http options; %w", err)
	}
	if rawServerIds != nil {
		ro.Servers = *rawServerIds
	}

	parallel, err := utils.GetNullableBoolean(valueMap, "parallel")
	if err != nil {
		return fmt.Errorf("invalid parallel in http options: %w", err)
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
