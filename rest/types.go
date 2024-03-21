package rest

import (
	"net/http"

	"github.com/hasura/ndc-rest-schema/schema"
)

const (
	defaultTimeout uint = 30

	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
)

// SchemaFile represents a schema file
type SchemaFile struct {
	Path        string                `json:"path" yaml:"path"`
	Spec        schema.SchemaSpecType `json:"spec" yaml:"spec"`
	MethodAlias map[string]string     `json:"methodAlias" yaml:"methodAlias"`
	TrimPrefix  string                `json:"trimPrefix" yaml:"trimPrefix"`
	EnvPrefix   string                `json:"envPrefix" yaml:"envPrefix"`
}

// Configuration contains required settings for the connector.
type Configuration struct {
	Files []SchemaFile `json:"files" yaml:"files"`
}

// State is the global state which is shared for every connector request.
type State struct {
	Schema *schema.NDCRestSchema
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
	schema *schema.NDCRestSchema
}
