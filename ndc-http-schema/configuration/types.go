package configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	restUtils "github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

var (
	errFilePathRequired   = errors.New("file path is empty")
	errHTTPMethodRequired = errors.New("the HTTP method is required")
)

var fieldNameRegex = regexp.MustCompile(`^[a-zA-Z_]\w+$`)

// Configuration contains required settings for the connector.
type Configuration struct {
	Output string `json:"output,omitempty" yaml:"output,omitempty"`
	// Require strict validation
	Strict         bool                   `json:"strict"         yaml:"strict"`
	ForwardHeaders ForwardHeadersSettings `json:"forwardHeaders" yaml:"forwardHeaders"`
	Concurrency    ConcurrencySettings    `json:"concurrency"    yaml:"concurrency"`
	Files          []ConfigItem           `json:"files"          yaml:"files"`
}

// ConcurrencySettings represent settings for concurrent webhook executions to remote servers.
type ConcurrencySettings struct {
	// Maximum number of concurrent executions if there are many query variables.
	Query uint `json:"query" yaml:"query"`
	// Maximum number of concurrent executions if there are many mutation operations.
	Mutation uint `json:"mutation" yaml:"mutation"`
	// Maximum number of concurrent requests to remote servers (distribution mode).
	HTTP uint `json:"http" yaml:"http"`
}

// ForwardHeadersSettings hold settings of header forwarding from and to Hasura engine
type ForwardHeadersSettings struct {
	// Enable headers forwarding.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// The argument field name to be added for headers forwarding.
	ArgumentField *string `json:"argumentField" jsonschema:"oneof_type=string;null,pattern=^[a-zA-Z_]\\w+$" yaml:"argumentField"`
	// HTTP response headers to be forwarded from a data connector to the client.
	ResponseHeaders *ForwardResponseHeadersSettings `json:"responseHeaders" jsonschema:"nullable" yaml:"responseHeaders"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ForwardHeadersSettings) UnmarshalJSON(b []byte) error {
	type Plain ForwardHeadersSettings
	var rawResult Plain
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	if !rawResult.Enabled {
		*j = ForwardHeadersSettings(rawResult)

		return nil
	}

	if rawResult.ArgumentField != nil && !fieldNameRegex.MatchString(*rawResult.ArgumentField) {
		return fmt.Errorf("invalid forwardHeaders.argumentField name format: %s", *rawResult.ArgumentField)
	}

	if rawResult.ResponseHeaders != nil {
		if err := rawResult.ResponseHeaders.Validate(); err != nil {
			return fmt.Errorf("responseHeaders: %w", err)
		}
	}

	*j = ForwardHeadersSettings(rawResult)

	return nil
}

// ForwardHeadersSettings hold settings of header forwarding from http response to Hasura engine.
type ForwardResponseHeadersSettings struct {
	// Name of the field in the NDC function/procedure's result which contains the response headers.
	HeadersField string `json:"headersField" jsonschema:"pattern=^[a-zA-Z_]\\w+$" yaml:"headersField"`
	// Name of the field in the NDC function/procedure's result which contains the result.
	ResultField string `json:"resultField" jsonschema:"pattern=^[a-zA-Z_]\\w+$" yaml:"resultField"`
	// List of actual HTTP response headers from the data connector to be set as response headers. Returns all headers if empty.
	ForwardHeaders []string `json:"forwardHeaders" yaml:"forwardHeaders"`
}

// Validate checks if the setting is valid.
func (j ForwardResponseHeadersSettings) Validate() error {
	if !fieldNameRegex.MatchString(j.HeadersField) {
		return fmt.Errorf("invalid format in headersField: %s", j.HeadersField)
	}

	if !fieldNameRegex.MatchString(j.ResultField) {
		return fmt.Errorf("invalid format in resultField: %s", j.ResultField)
	}

	return nil
}

// RetryPolicySetting represents retry policy settings
type RetryPolicySetting struct {
	// Number of retry times
	Times utils.EnvInt `json:"times,omitempty" mapstructure:"times" yaml:"times,omitempty"`
	// Delay retry delay in milliseconds
	Delay utils.EnvInt `json:"delay,omitempty" mapstructure:"delay" yaml:"delay,omitempty"`
	// HTTPStatus retries if the remote service returns one of these http status
	HTTPStatus []int `json:"httpStatus,omitempty" mapstructure:"httpStatus" yaml:"httpStatus,omitempty"`
}

// Validate if the current instance is valid
func (rs RetryPolicySetting) Validate() (*rest.RetryPolicy, error) {
	var errs []error
	times, err := rs.Times.Get()
	if err != nil {
		errs = append(errs, err)
	} else if times < 0 {
		errs = append(errs, errors.New("retry policy times must be positive"))
	}

	delay, err := rs.Delay.Get()
	if err != nil {
		errs = append(errs, err)
	} else if delay < 0 {
		errs = append(errs, errors.New("retry delay must be larger than 0"))
	}

	for _, status := range rs.HTTPStatus {
		if status < 400 || status >= 600 {
			errs = append(errs, errors.New("retry http status must be in between 400 and 599"))

			break
		}
	}

	result := &rest.RetryPolicy{
		Times:      uint(times),
		Delay:      uint(delay),
		HTTPStatus: rs.HTTPStatus,
	}

	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}

	return result, nil
}

// ConfigItem extends the ConvertConfig with advanced options
type ConfigItem struct {
	ConvertConfig `yaml:",inline"`

	// Distributed enables distributed schema
	Distributed *bool `json:"distributed,omitempty" yaml:"distributed,omitempty"`
	// configure the request timeout in seconds.
	Timeout *utils.EnvInt       `json:"timeout,omitempty" mapstructure:"timeout" yaml:"timeout,omitempty"`
	Retry   *RetryPolicySetting `json:"retry,omitempty"   mapstructure:"retry"   yaml:"retry,omitempty"`
}

// IsDistributed checks if the distributed option is enabled
func (ci ConfigItem) IsDistributed() bool {
	return ci.Distributed != nil && *ci.Distributed
}

// GetRuntimeSettings validate and get runtime settings
func (ci ConfigItem) GetRuntimeSettings() (*rest.RuntimeSettings, error) {
	result := &rest.RuntimeSettings{}
	var errs []error
	if ci.Timeout != nil {
		timeout, err := ci.Timeout.Get()
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("timeout: %w", err))
		case timeout < 0:
			errs = append(errs, fmt.Errorf("timeout must be positive, got: %d", timeout))
		default:
			result.Timeout = uint(timeout)
		}
	}

	if ci.Retry != nil {
		retryPolicy, err := ci.Retry.Validate()
		if err != nil {
			errs = append(errs, fmt.Errorf("ConfigItem.retry: %w", err))
		}
		result.Retry = *retryPolicy
	}

	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}

	return result, nil
}

// ConvertConfig represents the content of convert config file
type ConvertConfig struct {
	// File path needs to be converted
	File string `json:"file" jsonschema:"required" yaml:"file"`
	// The API specification of the file, is one of oas3 (openapi3), oas2 (openapi2)
	Spec rest.SchemaSpecType `json:"spec,omitempty" jsonschema:"default=oas3" yaml:"spec"`
	// Alias names for HTTP method. Used for prefix renaming, e.g. getUsers, postUser
	MethodAlias map[string]string `json:"methodAlias,omitempty" yaml:"methodAlias"`
	// Add a prefix to the function and procedure names
	Prefix string `json:"prefix,omitempty" yaml:"prefix"`
	// Trim the prefix in URL, e.g. /v1
	TrimPrefix string `json:"trimPrefix,omitempty" yaml:"trimPrefix"`
	// The environment variable prefix for security values, e.g. PET_STORE
	EnvPrefix string `json:"envPrefix,omitempty" yaml:"envPrefix"`
	// Return the pure NDC schema only
	Pure bool `json:"pure,omitempty" yaml:"pure"`
	// Require strict validation
	Strict bool `json:"strict,omitempty" yaml:"strict"`
	// Ignore deprecated fields.
	NoDeprecation bool `json:"noDeprecation,omitempty" yaml:"noDeprecation"`
	// Patch files to be applied into the input file before converting
	PatchBefore []restUtils.PatchConfig `json:"patchBefore,omitempty" yaml:"patchBefore"`
	// Patch files to be applied into the input file after converting
	PatchAfter []restUtils.PatchConfig `json:"patchAfter,omitempty" yaml:"patchAfter"`
	// Allowed content types. All content types are allowed by default
	AllowedContentTypes []string `json:"allowedContentTypes,omitempty" yaml:"allowedContentTypes"`
	// The location where the ndc schema file will be generated. Print to stdout if not set
	Output string `json:"output,omitempty" yaml:"output,omitempty"`
}

// NDCHttpRuntimeSchema wraps NDCHttpSchema with runtime settings
type NDCHttpRuntimeSchema struct {
	Name    string               `json:"name" yaml:"name"`
	Runtime rest.RuntimeSettings `json:"-"    yaml:"-"`
	*rest.NDCHttpSchema
}

// ConvertCommandArguments represent available command arguments for the convert command
type ConvertCommandArguments struct {
	File                string            `help:"File path needs to be converted."                                                     short:"f"`
	Config              string            `help:"Path of the config file."                                                             short:"c"`
	Output              string            `help:"The location where the ndc schema file will be generated. Print to stdout if not set" short:"o"`
	Spec                string            `help:"The API specification of the file, is one of oas3 (openapi3), oas2 (openapi2)"`
	Format              string            `default:"json"                                                                              help:"The output format, is one of json, yaml. If the output is set, automatically detect the format in the output file extension"`
	Strict              bool              `default:"false"                                                                             help:"Require strict validation"`
	NoDeprecation       bool              `default:"false"                                                                             help:"Ignore deprecated fields"`
	Pure                bool              `default:"false"                                                                             help:"Return the pure NDC schema only"`
	Prefix              string            `help:"Add a prefix to the function and procedure names"`
	TrimPrefix          string            `help:"Trim the prefix in URL, e.g. /v1"`
	EnvPrefix           string            `help:"The environment variable prefix for security values, e.g. PET_STORE"`
	MethodAlias         map[string]string `help:"Alias names for HTTP method. Used for prefix renaming, e.g. getUsers, postUser"`
	AllowedContentTypes []string          `help:"Allowed content types. All content types are allowed by default"`
	PatchBefore         []string          `help:"Patch files to be applied into the input file before converting"`
	PatchAfter          []string          `help:"Patch files to be applied into the input file after converting"`
}

// the object type of HTTP execution options for single server
var singleObjectType = rest.ObjectType{
	Description: utils.ToPtr("Execution options for HTTP requests to a single server"),
	Fields: map[string]rest.ObjectField{
		"servers": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr("Specify remote servers to receive the request. If there are many server IDs the server is selected randomly"),
				Type:        schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(rest.HTTPServerIDScalarName))).Encode(),
			},
		},
	},
}

// the object type of HTTP execution options for distributed servers
var distributedObjectType rest.ObjectType = rest.ObjectType{
	Description: utils.ToPtr("Distributed execution options for HTTP requests to multiple servers"),
	Fields: map[string]rest.ObjectField{
		"servers": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr("Specify remote servers to receive the request"),
				Type:        schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(rest.HTTPServerIDScalarName))).Encode(),
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

var httpSingleOptionsArgument = rest.ArgumentInfo{
	ArgumentInfo: schema.ArgumentInfo{
		Description: singleObjectType.Description,
		Type:        schema.NewNullableNamedType(rest.HTTPSingleOptionsObjectName).Encode(),
	},
}
