package rest

import (
	"errors"
	"net/http"

	"github.com/hasura/ndc-rest/ndc-rest-schema/command"
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/rest/internal"
)

var (
	errBuildSchemaFailed  = errors.New("failed to build NDC REST schema")
	errInvalidSchema      = errors.New("failed to validate NDC REST schema")
	errHTTPMethodRequired = errors.New("the HTTP method is required")
	errArgumentRequired   = errors.New("argument is required")
)

const (
	contentTypeJSON = "application/json"
)

// ConfigItem extends the ConvertConfig with advanced options
type ConfigItem struct {
	command.ConvertConfig `yaml:",inline"`

	// Distributed enables distributed schema
	Distributed bool `json:"distributed" yaml:"distributed"`
}

// Configuration contains required settings for the connector.
type Configuration struct {
	Files []ConfigItem `json:"files" yaml:"files"`
}

// State is the global state which is shared for every connector request.
type State struct {
	Schema *rest.NDCRestSchema
}

type options struct {
	client internal.Doer
}

var defaultOptions options = options{
	client: &http.Client{
		Transport: http.DefaultTransport,
	},
}

// Option is an interface to set custom REST connector options
type Option (func(*options))

// WithClient sets the custom HTTP client that satisfy the Doer interface
func WithClient(client internal.Doer) Option {
	return func(opts *options) {
		opts.client = client
	}
}

// NDCRestSchemaWithName wraps NDCRestSchema with identity name
type NDCRestSchemaWithName struct {
	name   string
	schema *rest.NDCRestSchema
}
