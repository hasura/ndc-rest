package rest

import (
	"errors"
	"net/http"

	"github.com/hasura/ndc-rest/connector/internal"
)

var (
	errInvalidSchema     = errors.New("failed to validate NDC REST schema")
	errBuildSchemaFailed = errors.New("failed to build NDC REST schema")
)

// State is the global state which is shared for every connector request.
type State struct{}

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
