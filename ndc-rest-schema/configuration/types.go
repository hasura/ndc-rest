package configuration

import (
	"github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/ndc-rest-schema/utils"
)

// ConfigItem extends the ConvertConfig with advanced options
type ConfigItem struct {
	ConvertConfig `yaml:",inline"`

	// Distributed enables distributed schema
	Distributed bool `json:"distributed" yaml:"distributed"`
}

// Configuration contains required settings for the connector.
type Configuration struct {
	Output string       `json:"output,omitempty" yaml:"output,omitempty"`
	Files  []ConfigItem `json:"files" yaml:"files"`
}

// ConvertConfig represents the content of convert config file
type ConvertConfig struct {
	// File path needs to be converted
	File string `json:"file" jsonschema:"required" yaml:"file"`
	// The API specification of the file, is one of oas3 (openapi3), oas2 (openapi2)
	Spec schema.SchemaSpecType `json:"spec,omitempty" jsonschema:"default=oas3" yaml:"spec"`
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
	PatchBefore []utils.PatchConfig `json:"patchBefore,omitempty" yaml:"patchBefore"`
	// Patch files to be applied into the input file after converting
	PatchAfter []utils.PatchConfig `json:"patchAfter,omitempty" yaml:"patchAfter"`
	// Allowed content types. All content types are allowed by default
	AllowedContentTypes []string `json:"allowedContentTypes,omitempty" yaml:"allowedContentTypes"`
	// The location where the ndc schema file will be generated. Print to stdout if not set
	Output string `json:"output,omitempty" yaml:"output"`
}
