package security

import (
	"fmt"
	"net/http"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
)

// APIKeyCredential presents an API key credential
type ApiKeyCredential struct {
	In    schema.APIKeyLocation
	Name  string
	Value string

	client *http.Client
}

// NewApiKeyCredential creates a new APIKeyCredential instance.
func NewApiKeyCredential(client *http.Client, config *schema.APIKeyAuthConfig) (*ApiKeyCredential, error) {
	value, err := config.Value.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to create ApiKeyCredential: %w", err)
	}

	return &ApiKeyCredential{
		In:    config.In,
		Name:  config.Name,
		Value: value,

		client: client,
	}, nil
}

// GetClient gets the HTTP client that is compatible with the current credential.
func (akc ApiKeyCredential) GetClient() *http.Client {
	return akc.client
}

// Inject the credential into the incoming request
func (akc ApiKeyCredential) Inject(req *http.Request) (bool, error) {
	if akc.Value == "" {
		return false, nil
	}

	akc.inject(req, akc.Value)

	return true, nil
}

// InjectMock injects the mock credential into the incoming request for explain APIs.
func (akc ApiKeyCredential) InjectMock(req *http.Request) bool {
	if akc.Value == "" {
		return false
	}

	akc.inject(req, utils.MaskString(akc.Value))

	return true
}

func (akc ApiKeyCredential) inject(req *http.Request, value string) {
	switch akc.In {
	case schema.APIKeyInHeader:
		req.Header.Set(akc.Name, value)
	case schema.APIKeyInQuery:
		endpoint := req.URL
		q := endpoint.Query()
		q.Add(akc.Name, value)
		endpoint.RawQuery = q.Encode()
		req.URL = endpoint
	}
}
