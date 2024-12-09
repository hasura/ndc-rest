package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/hasura/ndc-http/connector/internal/argument"
	"github.com/hasura/ndc-http/connector/internal/security"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/utils"
)

// UpstreamManager represents a manager for an upstream.
type UpstreamManager struct {
	config        *configuration.Configuration
	defaultClient *http.Client
	upstreams     map[string]UpstreamSetting
}

// NewUpstreamManager creates a new UpstreamManager instance.
func NewUpstreamManager(httpClient *http.Client, config *configuration.Configuration) *UpstreamManager {
	return &UpstreamManager{
		config:        config,
		defaultClient: httpClient,
		upstreams:     make(map[string]UpstreamSetting),
	}
}

// Register evaluates and registers an upstream from config.
func (um *UpstreamManager) Register(ctx context.Context, runtimeSchema *configuration.NDCHttpRuntimeSchema, ndcSchema *schema.NDCHttpSchema) error {
	logger := connector.GetLogger(ctx)
	namespace := runtimeSchema.Name
	httpClient := um.defaultClient

	if runtimeSchema.Settings.TLS != nil {
		tlsClient, err := security.NewHTTPClientTLS(httpClient, runtimeSchema.Settings.TLS)
		if err != nil {
			return fmt.Errorf("%s: %w", namespace, err)
		}

		if tlsClient != nil {
			httpClient = tlsClient
		}
	}

	settings := UpstreamSetting{
		servers:     make(map[string]Server),
		security:    runtimeSchema.Settings.Security,
		headers:     um.getHeadersFromEnv(logger, namespace, runtimeSchema.Settings.Headers),
		credentials: um.registerSecurityCredentials(ctx, httpClient, runtimeSchema.Settings.SecuritySchemes, logger.With(slog.String("namespace", namespace))),
		httpClient:  httpClient,
	}

	if len(runtimeSchema.Settings.ArgumentPresets) > 0 {
		argumentPresets, err := argument.NewArgumentPresets(ndcSchema, runtimeSchema.Settings.ArgumentPresets)
		if err != nil {
			return fmt.Errorf("%s: %w", namespace, err)
		}
		settings.argumentPresets = argumentPresets
	}

	for i, server := range runtimeSchema.Settings.Servers {
		serverID := server.ID
		if serverID == "" {
			serverID = strconv.Itoa(i)
		}

		serverURL, err := server.GetURL()
		if err != nil {
			// Relax the error to allow schema introspection without environment variables setting.
			// Moreover, because there are many security schemes the user may use one of them.
			logger.Error(fmt.Sprintf("failed to register server %s:%s, %s", namespace, serverID, err))

			continue
		}

		serverClient := httpClient
		if server.TLS != nil {
			tlsClient, err := security.NewHTTPClientTLS(um.defaultClient, server.TLS)
			if err != nil {
				return fmt.Errorf("%s.server[%s]: %w", namespace, serverID, err)
			}

			if tlsClient != nil {
				serverClient = tlsClient
			}
		}

		newServer := Server{
			URL:         serverURL,
			Headers:     um.getHeadersFromEnv(logger, namespace, server.Headers),
			Security:    server.Security,
			Credentials: um.registerSecurityCredentials(ctx, serverClient, server.SecuritySchemes, logger.With(slog.String("namespace", namespace), slog.String("server_id", serverID))),
			HTTPClient:  serverClient,
		}

		if len(server.ArgumentPresets) > 0 {
			argumentPresets, err := argument.NewArgumentPresets(ndcSchema, server.ArgumentPresets)
			if err != nil {
				return fmt.Errorf("%s.server[%s]: %w", namespace, serverID, err)
			}
			newServer.ArgumentPresets = argumentPresets
		}

		settings.servers[serverID] = newServer
	}

	um.upstreams[namespace] = settings

	return nil
}

// ExecuteRequest executes a request to the upstream server.
func (um *UpstreamManager) ExecuteRequest(ctx context.Context, request *RetryableRequest, namespace string) (*http.Response, context.CancelFunc, error) {
	req, cancel, err := request.CreateRequest(ctx)
	if err != nil {
		return nil, nil, err
	}

	httpClient, err := um.evalRequestSettings(ctx, request, req, namespace)
	if err != nil {
		cancel()

		return nil, nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		cancel()

		return nil, nil, err
	}

	return resp, cancel, nil
}

func (um *UpstreamManager) evalRequestSettings(ctx context.Context, request *RetryableRequest, req *http.Request, namespace string) (*http.Client, error) {
	httpClient := um.defaultClient
	settings, ok := um.upstreams[namespace]
	if !ok {
		return um.defaultClient, nil
	}
	if settings.httpClient != nil {
		httpClient = settings.httpClient
	}

	for key, header := range settings.headers {
		req.Header.Set(key, header)
	}

	securities := request.RawRequest.Security
	if len(securities) == 0 {
		securities = settings.security
	}

	logger := connector.GetLogger(ctx)
	securityOptional := securities.IsOptional()

	var err error
	server, ok := settings.servers[request.ServerID]
	if ok {
		if server.HTTPClient != nil {
			httpClient = server.HTTPClient
		}

		for key, header := range server.Headers {
			if header != "" {
				req.Header.Set(key, header)
			}
		}

		if !securityOptional && len(server.Credentials) > 0 {
			var hc *http.Client
			hc, err = um.evalSecuritySchemes(req, securities, server.Credentials)
			if err != nil {
				logger.Error(fmt.Sprintf("failed to evaluate the authentication: %s", err), slog.String("namespace", namespace), slog.String("server_id", request.ServerID))
			}

			if hc != nil {
				return hc, nil
			}
		}
	}

	if !securityOptional && len(settings.credentials) > 0 {
		hc, err := um.evalSecuritySchemes(req, securities, settings.credentials)
		if err != nil {
			logger.Error(fmt.Sprintf("failed to evaluate the authentication: %s", err), slog.String("namespace", namespace))

			return nil, err
		}

		if hc != nil {
			return hc, nil
		}
	}

	return httpClient, nil
}

func (um *UpstreamManager) evalSecuritySchemes(req *http.Request, securities rest.AuthSecurities, credentials map[string]security.Credential) (*http.Client, error) {
	for _, security := range securities {
		sc, ok := credentials[security.Name()]
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

	return nil, nil
}

// InjectMockCredential injects mock credential into the request for explain APIs.
func (um *UpstreamManager) InjectMockRequestSettings(req *http.Request, namespace string, securities rest.AuthSecurities) {
	settings, ok := um.upstreams[namespace]
	if !ok {
		return
	}

	for key, header := range settings.headers {
		req.Header.Set(key, header)
	}

	if len(securities) == 0 {
		securities = settings.security
	}

	if securities.IsOptional() || len(settings.credentials) == 0 {
		return
	}

	for _, security := range securities {
		sc, ok := settings.credentials[security.Name()]
		if !ok {
			continue
		}
		hasAuth := sc.InjectMock(req)
		if hasAuth {
			return
		}
	}
}

func (um *UpstreamManager) getHeadersFromEnv(logger *slog.Logger, namespace string, headers map[string]utils.EnvString) map[string]string {
	results := make(map[string]string)
	for key, header := range headers {
		value, err := header.Get()
		if err != nil {
			logger.Error(err.Error(), slog.String("namespace", namespace), slog.String("header", key))
		} else if value != "" {
			results[key] = value
		}
	}

	return results
}

func (um *UpstreamManager) registerSecurityCredentials(ctx context.Context, httpClient *http.Client, securitySchemes map[string]rest.SecurityScheme, logger *slog.Logger) map[string]security.Credential {
	credentials := make(map[string]security.Credential)

	for key, ss := range securitySchemes {
		cred, headerForwardRequired, err := security.NewCredential(ctx, httpClient, ss)
		if err != nil {
			// Relax the error to allow schema introspection without environment variables setting.
			// Moreover, because there are many security schemes the user may use one of them.
			logger.Error(
				fmt.Sprintf("failed to register security scheme %s, %s", key, err),
				slog.String("scheme", key),
			)

			continue
		}

		credentials[key] = cred
		if headerForwardRequired && (!um.config.ForwardHeaders.Enabled || um.config.ForwardHeaders.ArgumentField == nil || *um.config.ForwardHeaders.ArgumentField == "") {
			logger.Warn("the security scheme needs header forwarding enabled with argumentField set", slog.String("scheme", key))
		}
	}

	return credentials
}
