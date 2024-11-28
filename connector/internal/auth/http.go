package auth

import (
	"fmt"
	"net/http"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
)

// HTTPCredential presents a header authentication credential
type HTTPCredential struct {
	Header string
	Scheme string
	Value  string

	client *http.Client
}

var _ Credential = &HTTPCredential{}

// NewHTTPCredential creates a new HTTPCredential instance.
func NewHTTPCredential(client *http.Client, config *schema.HTTPAuthConfig) (*HTTPCredential, error) {
	value, err := config.Value.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to create ApiKeyCredential: %w", err)
	}

	return &HTTPCredential{
		Header: config.Header,
		Scheme: config.Scheme,
		Value:  value,
		client: client,
	}, nil
}

// GetClient gets the HTTP client that is compatible with the current credential.
func (hc HTTPCredential) GetClient() *http.Client {
	return hc.client
}

// Inject the credential into the incoming request
func (hc HTTPCredential) Inject(req *http.Request) (bool, error) {
	if hc.Value == "" {
		return false, nil
	}

	hc.inject(req, hc.Value)

	return true, nil
}

// InjectMock injects the mock credential into the incoming request for explain APIs.
func (hc HTTPCredential) InjectMock(req *http.Request) bool {
	if hc.Value == "" {
		return false
	}

	hc.inject(req, utils.MaskString(hc.Value))

	return true
}

func (hc HTTPCredential) inject(req *http.Request, value string) {
	headerName := hc.Header
	if headerName == "" {
		headerName = schema.AuthorizationHeader
	}
	scheme := hc.Scheme
	if scheme == "bearer" {
		scheme = "Bearer"
	}

	req.Header.Set(headerName, scheme+" "+value)
}

// CookieCredential presents a cookie credential
type CookieCredential struct {
	client *http.Client
}

var _ Credential = &CookieCredential{}

// NewCookieCredential creates a new CookieCredential instance.
func NewCookieCredential(client *http.Client) (*CookieCredential, error) {
	return &CookieCredential{
		client: client,
	}, nil
}

// GetClient gets the HTTP client that is compatible with the current credential.
func (cc CookieCredential) GetClient() *http.Client {
	return cc.client
}

// Inject the credential into the incoming request
func (cc CookieCredential) Inject(req *http.Request) (bool, error) {
	return false, nil
}

// InjectMock injects the mock credential into the incoming request for explain APIs.
func (cc CookieCredential) InjectMock(req *http.Request) bool {
	return false
}
