package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/hasura/ndc-http/connector/internal/auth"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// Server contains server settings.
type Server struct {
	URL         *url.URL
	Headers     map[string]string
	Credentials map[string]auth.Credential
	Security    rest.AuthSecurities
}

type UpstreamSetting struct {
	servers     map[string]Server
	headers     map[string]string
	security    rest.AuthSecurities
	credentials map[string]auth.Credential
}

type UpstreamManager struct {
	defaultClient *http.Client
	httpClients   map[string]*http.Client
	upstreams     map[string]UpstreamSetting
}

func NewUpstreamManager(httpClient *http.Client) *UpstreamManager {
	return &UpstreamManager{
		defaultClient: httpClient,
		httpClients:   make(map[string]*http.Client),
		upstreams:     make(map[string]UpstreamSetting),
	}
}

func (sm *UpstreamManager) Register(ctx context.Context, namespace string, rawSettings *rest.NDCHttpSettings) error {
	logger := connector.GetLogger(ctx)
	httpClient := sm.defaultClient
	sm.httpClients[namespace] = httpClient

	settings := UpstreamSetting{
		servers:     make(map[string]Server),
		security:    rawSettings.Security,
		headers:     sm.getHeadersFromEnv(logger, namespace, rawSettings.Headers),
		credentials: make(map[string]auth.Credential),
	}

	for key, ss := range rawSettings.SecuritySchemes {
		cred, err := auth.NewCredential(ctx, httpClient, ss)
		if err != nil {
			// Relax the error to allow schema introspection without environment variables setting.
			// Moreover, because there are many security schemes the user may use one of them.
			logger.Error(
				fmt.Sprintf("failed to register security scheme %s:%s, %s", namespace, key, err),
				slog.String("namespace", namespace),
				slog.String("scheme", key),
			)
		} else {
			settings.credentials[key] = cred
		}
	}

	for i, server := range rawSettings.Servers {
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

		newServer := Server{
			URL:         serverURL,
			Headers:     sm.getHeadersFromEnv(logger, namespace, server.Headers),
			Credentials: make(map[string]auth.Credential),
			Security:    server.Security,
		}

		for key, ss := range server.SecuritySchemes {
			cred, err := auth.NewCredential(ctx, httpClient, ss)
			if err != nil {
				// Relax the error to allow schema introspection without environment variables setting.
				// Moreover, because there are many security schemes the user may use one of them.
				logger.Error(
					fmt.Sprintf("failed to register security scheme for server %s: %s", serverID, err),
					slog.String("namespace", namespace),
					slog.String("scheme", key),
				)
			} else {
				newServer.Credentials[key] = cred
			}
		}

		settings.servers[serverID] = newServer
	}

	sm.upstreams[namespace] = settings

	return nil
}

func (sm *UpstreamManager) ExecuteRequest(ctx context.Context, request *RetryableRequest, namespace string) (*http.Response, context.CancelFunc, error) {
	req, cancel, err := request.CreateRequest(ctx)
	if err != nil {
		return nil, nil, err
	}

	httpClient, err := sm.evalRequestSettings(ctx, request, req, namespace)
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

func (sm *UpstreamManager) evalRequestSettings(ctx context.Context, request *RetryableRequest, req *http.Request, namespace string) (*http.Client, error) {
	httpClient, ok := sm.httpClients[namespace]
	if !ok {
		httpClient = sm.defaultClient
	}

	settings, ok := sm.upstreams[namespace]
	if !ok {
		return httpClient, nil
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
		for key, header := range server.Headers {
			if header != "" {
				req.Header.Set(key, header)
			}
		}

		if !securityOptional && len(server.Credentials) > 0 {
			var hc *http.Client
			hc, err = sm.evalSecuritySchemes(req, securities, server.Credentials)
			if err != nil {
				logger.Error(fmt.Sprintf("failed to evaluate the authentication: %s", err), slog.String("namespace", namespace), slog.String("server_id", request.ServerID))
			}

			if hc != nil {
				return hc, nil
			}
		}
	}

	if !securityOptional && len(settings.credentials) > 0 {
		hc, err := sm.evalSecuritySchemes(req, securities, settings.credentials)
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

func (sm *UpstreamManager) evalSecuritySchemes(req *http.Request, securities rest.AuthSecurities, credentials map[string]auth.Credential) (*http.Client, error) {
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
func (sm *UpstreamManager) InjectMockRequestSettings(req *http.Request, namespace string, securities rest.AuthSecurities) {
	settings, ok := sm.upstreams[namespace]
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

// BuildDistributedRequestsWithOptions builds distributed requests with options
func (sm *UpstreamManager) BuildDistributedRequestsWithOptions(request *RetryableRequest, httpOptions *HTTPOptions) ([]RetryableRequest, error) {
	if strings.HasPrefix(request.URL.Scheme, "http") {
		return []RetryableRequest{*request}, nil
	}

	upstream, ok := sm.upstreams[request.Namespace]
	if !ok {
		return nil, schema.InternalServerError(fmt.Sprintf("upstream with namespace %s does not exist", request.Namespace), nil)
	}

	if len(upstream.servers) == 0 {
		return nil, schema.InternalServerError("no available server in the upstream with namespace "+request.Namespace, nil)
	}

	if !httpOptions.Distributed || len(upstream.servers) == 1 {
		baseURL, serverID, err := sm.getBaseURLFromServers(upstream.servers, request.Namespace, httpOptions.Servers)
		if err != nil {
			return nil, err
		}

		request.URL.Scheme = baseURL.Scheme
		request.URL.Host = baseURL.Host
		request.URL.Path = path.Join(baseURL.Path, request.URL.Path)
		request.ServerID = serverID

		return []RetryableRequest{*request}, nil
	}

	var requests []RetryableRequest
	var buf []byte
	var err error
	if httpOptions.Parallel && request.Body != nil {
		// copy new readers for each requests to avoid race condition
		buf, err = io.ReadAll(request.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}
	serverIDs := httpOptions.Servers
	if len(serverIDs) == 0 {
		serverIDs = utils.GetKeys(upstream.servers)
	}

	for _, serverID := range serverIDs {
		baseURL, serverID, err := sm.getBaseURLFromServers(upstream.servers, request.Namespace, []string{serverID})
		if err != nil {
			return nil, err
		}
		baseURL.Path += request.URL.Path
		baseURL.RawQuery = request.URL.RawQuery
		baseURL.Fragment = request.URL.Fragment
		req := RetryableRequest{
			URL:         *baseURL,
			ServerID:    serverID,
			RawRequest:  request.RawRequest,
			ContentType: request.ContentType,
			Headers:     request.Headers.Clone(),
			Body:        request.Body,
		}
		if len(buf) > 0 {
			req.Body = bytes.NewReader(buf)
		}
		requests = append(requests, req)
	}

	return requests, nil
}

func (sm *UpstreamManager) getHeadersFromEnv(logger *slog.Logger, namespace string, headers map[string]utils.EnvString) map[string]string {
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

func (sm *UpstreamManager) getBaseURLFromServers(servers map[string]Server, namespace string, serverIDs []string) (*url.URL, string, error) {
	var results []*url.URL
	var selectedServerIDs []string
	for key, server := range servers {
		if len(serverIDs) > 0 && !slices.Contains(serverIDs, key) {
			continue
		}

		hostPtr := server.URL
		results = append(results, hostPtr)
		selectedServerIDs = append(selectedServerIDs, key)
	}

	switch len(results) {
	case 0:
		return nil, "", fmt.Errorf("requested servers %v in the upstream with namespace %s do not exist", serverIDs, namespace)
	case 1:
		result := results[0]

		return result, selectedServerIDs[0], nil
	default:
		index := rand.IntN(len(results) - 1)
		host := results[index]

		return host, selectedServerIDs[index], nil
	}
}
