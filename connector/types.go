package connector

import (
	"errors"
	"net/http"

	"github.com/hasura/ndc-http/connector/internal"
	"github.com/hasura/ndc-sdk-go/connector"
)

var (
	errInvalidSchema     = errors.New("failed to validate NDC HTTP schema")
	errBuildSchemaFailed = errors.New("failed to build NDC HTTP schema")
)

// State is the global state which is shared for every connector request.
type State struct {
	Tracer *connector.Tracer
}

type options struct {
	client internal.Doer
}

var defaultOptions options = options{
	client: &http.Client{
		Transport: http.DefaultTransport,
	},
}

// Option is an interface to set custom HTTP connector options
type Option (func(*options))

// WithClient sets the custom HTTP client that satisfy the Doer interface
func WithClient(client internal.Doer) Option {
	return func(opts *options) {
		opts.client = client
	}
}
