package configuration

import (
	"errors"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	restUtils "github.com/hasura/ndc-rest/ndc-rest-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

var (
	errFilePathRequired   = errors.New("file path is empty")
	errHTTPMethodRequired = errors.New("the HTTP method is required")
)

// ConfigItem extends the ConvertConfig with advanced options
type ConfigItem struct {
	ConvertConfig `yaml:",inline"`

	// Distributed enables distributed schema
	Distributed bool `json:"distributed" yaml:"distributed"`
}

// Configuration contains required settings for the connector.
type Configuration struct {
	Output string `json:"output,omitempty" yaml:"output,omitempty"`
	// Require strict validation
	Strict         bool                   `json:"strict" yaml:"strict"`
	ForwardHeaders ForwardHeadersSettings `json:"forwardHeaders" yaml:"forwardHeaders"`
	Files          []ConfigItem           `json:"files" yaml:"files"`
}

// ForwardHeadersSettings hold settings of header forwarding from Hasura engine
type ForwardHeadersSettings struct {
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	ArgumentName string `json:"argumentName" yaml:"argumentName"`
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
	// Patch files to be applied into the input file before converting
	PatchBefore []restUtils.PatchConfig `json:"patchBefore,omitempty" yaml:"patchBefore"`
	// Patch files to be applied into the input file after converting
	PatchAfter []restUtils.PatchConfig `json:"patchAfter,omitempty" yaml:"patchAfter"`
	// Allowed content types. All content types are allowed by default
	AllowedContentTypes []string `json:"allowedContentTypes,omitempty" yaml:"allowedContentTypes"`
	// The location where the ndc schema file will be generated. Print to stdout if not set
	Output string `json:"output,omitempty" yaml:"output,omitempty"`
}

// NDCRestSchemaWithName wraps NDCRestSchema with identity name
type NDCRestSchemaWithName struct {
	Name string `json:"name" yaml:"name"`
	*rest.NDCRestSchema
}

// ConvertCommandArguments represent available command arguments for the convert command
type ConvertCommandArguments struct {
	File                string            `help:"File path needs to be converted."                                                     short:"f"`
	Config              string            `help:"Path of the config file."                                                             short:"c"`
	Output              string            `help:"The location where the ndc schema file will be generated. Print to stdout if not set" short:"o"`
	Spec                string            `help:"The API specification of the file, is one of oas3 (openapi3), oas2 (openapi2)"`
	Format              string            `default:"json"                                                                              help:"The output format, is one of json, yaml. If the output is set, automatically detect the format in the output file extension"`
	Strict              bool              `default:"false"                                                                             help:"Require strict validation"`
	Pure                bool              `default:"false"                                                                             help:"Return the pure NDC schema only"`
	Prefix              string            `help:"Add a prefix to the function and procedure names"`
	TrimPrefix          string            `help:"Trim the prefix in URL, e.g. /v1"`
	EnvPrefix           string            `help:"The environment variable prefix for security values, e.g. PET_STORE"`
	MethodAlias         map[string]string `help:"Alias names for HTTP method. Used for prefix renaming, e.g. getUsers, postUser"`
	AllowedContentTypes []string          `help:"Allowed content types. All content types are allowed by default"`
	PatchBefore         []string          `help:"Patch files to be applied into the input file before converting"`
	PatchAfter          []string          `help:"Patch files to be applied into the input file after converting"`
}

// the object type of REST execution options for single server
var singleObjectType = rest.ObjectType{
	Description: utils.ToPtr("Execution options for REST requests to a single server"),
	Fields: map[string]rest.ObjectField{
		"servers": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr("Specify remote servers to receive the request. If there are many server IDs the server is selected randomly"),
				Type:        schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(rest.RESTServerIDScalarName))).Encode(),
			},
		},
	},
}

// the object type of REST execution options for distributed servers
var distributedObjectType rest.ObjectType = rest.ObjectType{
	Description: utils.ToPtr("Distributed execution options for REST requests to multiple servers"),
	Fields: map[string]rest.ObjectField{
		"servers": {
			ObjectField: schema.ObjectField{
				Description: utils.ToPtr("Specify remote servers to receive the request"),
				Type:        schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(rest.RESTServerIDScalarName))).Encode(),
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
