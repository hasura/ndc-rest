package rest

import (
	"errors"
	"net/http"

	"github.com/hasura/ndc-rest/connector/internal"
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
)

var (
	errInvalidSchema      = errors.New("failed to validate NDC REST schema")
	errBuildSchemaFailed  = errors.New("failed to build NDC REST schema")
	errHTTPMethodRequired = errors.New("the HTTP method is required")
	errFilePathRequired   = errors.New("file path is empty")
)

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
