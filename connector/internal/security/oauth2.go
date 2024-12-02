package security

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// OAuth2Client represent the client of the OAuth2 client credentials
type OAuth2Client struct {
	client  *http.Client
	isEmpty bool
}

var _ Credential = &OAuth2Client{}

// NewOAuth2Client creates an OAuth2 client from the security scheme
func NewOAuth2Client(ctx context.Context, httpClient *http.Client, flowType schema.OAuthFlowType, config *schema.OAuthFlow) (*OAuth2Client, error) {
	if flowType != schema.ClientCredentialsFlow || config.TokenURL == nil || config.ClientID == nil || config.ClientSecret == nil {
		return &OAuth2Client{
			client:  httpClient,
			isEmpty: true,
		}, nil
	}

	tokenURL, err := config.TokenURL.Get()
	if err != nil {
		return nil, fmt.Errorf("tokenUrl: %w", err)
	}

	if _, err := schema.ParseRelativeOrHttpURL(tokenURL); err != nil {
		return nil, fmt.Errorf("tokenUrl: %w", err)
	}

	scopes := make([]string, 0, len(config.Scopes))
	for scope := range config.Scopes {
		scopes = append(scopes, scope)
	}

	clientID, err := config.ClientID.Get()
	if err != nil {
		return nil, fmt.Errorf("clientId: %w", err)
	}

	clientSecret, err := config.ClientSecret.Get()
	if err != nil {
		return nil, fmt.Errorf("clientSecret: %w", err)
	}

	var endpointParams url.Values
	for key, envValue := range config.EndpointParams {
		value, err := envValue.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("endpointParams[%s]: %w", key, err)
		}
		if value != "" {
			endpointParams.Set(key, value)
		}
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	conf := &clientcredentials.Config{
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		Scopes:         scopes,
		TokenURL:       tokenURL,
		EndpointParams: endpointParams,
	}

	client := conf.Client(ctx)

	return &OAuth2Client{
		client: client,
	}, nil
}

// GetClient gets the HTTP client that is compatible with the current credential.
func (oc OAuth2Client) GetClient() *http.Client {
	return oc.client
}

// Inject the credential into the incoming request
func (oc OAuth2Client) Inject(req *http.Request) (bool, error) {
	return !oc.isEmpty, nil
}

// InjectMock injects the mock credential into the incoming request for explain APIs.
func (oc OAuth2Client) InjectMock(req *http.Request) bool {
	if oc.isEmpty {
		return false
	}

	req.Header.Set(schema.AuthorizationHeader, "Bearer xxx")

	return true
}
