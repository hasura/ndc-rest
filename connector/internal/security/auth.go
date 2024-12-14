package security

import (
	"context"
	"errors"
	"net/http"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
)

// Credential abstracts an authentication credential interface.
type Credential interface {
	// GetClient gets the HTTP client that is compatible with the current credential.
	GetClient() *http.Client
	// Inject the credential into the incoming request.
	Inject(request *http.Request) (bool, error)
	// InjectMock injects the mock credential into the incoming request for explain APIs.
	InjectMock(request *http.Request) bool
}

// NewCredential creates a generic credential from the security scheme.
func NewCredential(ctx context.Context, httpClient *http.Client, security schema.SecurityScheme) (Credential, bool, error) {
	if security.SecuritySchemer == nil {
		return nil, false, errors.New("empty security scheme")
	}

	switch ss := security.SecuritySchemer.(type) {
	case *schema.APIKeyAuthConfig:
		cred, err := NewApiKeyCredential(httpClient, ss)

		return cred, err != nil, err
	case *schema.BasicAuthConfig:
		cred, err := NewBasicCredential(httpClient, ss)

		return cred, err != nil, err
	case *schema.HTTPAuthConfig:
		cred, err := NewHTTPCredential(httpClient, ss)

		return cred, err != nil, err
	case *schema.OAuth2Config:
		var headerForwardingRequired bool
		for flowType, flow := range ss.Flows {
			if flowType != schema.ClientCredentialsFlow {
				headerForwardingRequired = true
			}

			cred, err := NewOAuth2Client(ctx, httpClient, flowType, &flow)

			return cred, headerForwardingRequired || err != nil, err
		}
	case *schema.CookieAuthConfig:
		cred, err := NewCookieCredential(httpClient)

		return cred, true, err
	case *schema.MutualTLSAuthConfig:
		return NewNoopCredential(httpClient), false, nil
	}

	return NewNoopCredential(httpClient), true, nil
}

// NoopCredential implements a no-op credential.
type NoopCredential struct {
	client *http.Client
}

var _ Credential = &NoopCredential{}

// NewNoopCredential creates a new NoopCredential instance.
func NewNoopCredential(client *http.Client) *NoopCredential {
	return &NoopCredential{
		client: client,
	}
}

// GetClient gets the HTTP client that is compatible with the current credential.
func (cc NoopCredential) GetClient() *http.Client {
	return cc.client
}

// Inject the credential into the incoming request
func (cc NoopCredential) Inject(req *http.Request) (bool, error) {
	return false, nil
}

// InjectMock injects the mock credential into the incoming request for explain APIs.
func (cc NoopCredential) InjectMock(req *http.Request) bool {
	return false
}
