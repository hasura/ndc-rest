package settings

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hasura/ndc-http/connector/internal/settings/auth"
	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/connector"
)

type SettingManager struct {
	defaultClient *http.Client
	httpClients   map[string]*http.Client
	credentials   map[string]auth.Credential
}

func NewSettingManager(httpClient *http.Client) *SettingManager {
	return &SettingManager{
		defaultClient: httpClient,
		httpClients:   make(map[string]*http.Client),
		credentials:   make(map[string]auth.Credential),
	}
}

func (sm *SettingManager) Register(ctx context.Context, namespace string, settings *schema.NDCHttpSettings) error {
	logger := connector.GetLogger(ctx)
	sm.httpClients[namespace] = sm.defaultClient

	for key, ss := range settings.SecuritySchemes {
		cred, err := auth.NewCredential(ctx, sm.defaultClient, ss)
		if err != nil {
			// Relax the error to allow schema introspection without environment variables setting.
			// Moreover, because there are many security schemes the user may use one of them.
			logger.Error(fmt.Sprintf("failed to register security scheme %s:%s, %s", namespace, key, err))
		} else {
			sm.credentials[namespace+":"+key] = cred
		}
	}

	return nil
}

func (sm *SettingManager) InjectCredential(req *http.Request, namespace string, securities schema.AuthSecurities) (*http.Client, error) {
	httpClient, ok := sm.httpClients[namespace]
	if !ok {
		httpClient = sm.defaultClient
	}

	if securities.IsOptional() || len(sm.credentials) == 0 {
		return httpClient, nil
	}

	for _, security := range securities {
		credentialKey := buildCredentialKey(namespace, security.Name())
		sc, ok := sm.credentials[credentialKey]
		if !ok {
			continue
		}

		hasAuth, err := sc.Inject(req)
		if err != nil {
			return nil, err
		}

		if hasAuth {
			return sc.GetClient(), nil
		}
	}

	return httpClient, nil
}

// InjectMockCredential injects mock credential into the request for explain APIs.
func (sm *SettingManager) InjectMockCredential(req *http.Request, namespace string, securities schema.AuthSecurities) {
	if securities.IsOptional() || len(sm.credentials) == 0 {
		return
	}

	for _, security := range securities {
		credentialKey := buildCredentialKey(namespace, security.Name())
		sc, ok := sm.credentials[credentialKey]
		if !ok {
			continue
		}

		hasAuth := sc.InjectMock(req)
		if hasAuth {
			return
		}
	}
}

func buildCredentialKey(namespace, key string) string {
	return namespace + ":" + key
}
