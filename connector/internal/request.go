package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/ndc-rest-schema/utils"
)

// RetryableRequest wraps the raw request with retryable
type RetryableRequest struct {
	RawRequest  *rest.Request
	URL         string
	ServerID    string
	ContentType string
	Headers     http.Header
	Body        io.ReadSeeker
	Runtime     rest.RuntimeSettings
}

// CreateRequest creates an HTTP request with body copied
func (r *RetryableRequest) CreateRequest(ctx context.Context) (*http.Request, context.CancelFunc, error) {
	if r.Body != nil {
		_, err := r.Body.Seek(0, io.SeekStart)
		if err != nil {
			return nil, nil, err
		}
	}

	timeout := r.Runtime.Timeout
	if timeout == 0 {
		timeout = defaultTimeoutSeconds
	}
	ctxR, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	request, err := http.NewRequestWithContext(ctxR, strings.ToUpper(r.RawRequest.Method), r.URL, r.Body)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	for key, header := range r.Headers {
		request.Header[key] = header
	}
	request.Header.Set(rest.ContentTypeHeader, r.ContentType)

	return request, cancel, nil
}

func getHostFromServers(servers []rest.ServerConfig, serverIDs []string) (string, string) {
	var results []string
	var selectedServerIDs []string
	for _, server := range servers {
		if len(serverIDs) > 0 && !slices.Contains(serverIDs, server.ID) {
			continue
		}
		hostPtr := server.GetURL()
		if hostPtr != "" {
			results = append(results, hostPtr)
			selectedServerIDs = append(selectedServerIDs, server.ID)
		}
	}

	switch len(results) {
	case 0:
		return "", ""
	case 1:
		return results[0], selectedServerIDs[0]
	default:
		index := rand.IntN(len(results) - 1)
		return results[index], selectedServerIDs[index]
	}
}

// BuildDistributedRequestsWithOptions builds distributed requests with options
func BuildDistributedRequestsWithOptions(request *RetryableRequest, restOptions *RESTOptions) ([]RetryableRequest, error) {
	if strings.HasPrefix(request.URL, "http") {
		return []RetryableRequest{*request}, nil
	}

	if !restOptions.Distributed || len(restOptions.Settings.Servers) == 1 {
		host, serverID := getHostFromServers(restOptions.Settings.Servers, restOptions.Servers)
		request.URL = host + request.URL
		request.ServerID = serverID
		if err := request.applySettings(restOptions.Settings, restOptions.Explain); err != nil {
			return nil, err
		}
		return []RetryableRequest{*request}, nil
	}

	var requests []RetryableRequest
	var buf []byte
	var err error
	if restOptions.Parallel && request.Body != nil {
		// copy new readers for each requests to avoid race condition
		buf, err = io.ReadAll(request.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}
	serverIDs := restOptions.Servers
	if len(serverIDs) == 0 {
		for _, server := range restOptions.Settings.Servers {
			serverIDs = append(serverIDs, server.ID)
		}
	}
	for _, serverID := range serverIDs {
		host, serverID := getHostFromServers(restOptions.Settings.Servers, []string{serverID})
		if host == "" {
			continue
		}

		req := RetryableRequest{
			URL:         host + request.URL,
			ServerID:    serverID,
			RawRequest:  request.RawRequest,
			ContentType: request.ContentType,
			Headers:     request.Headers.Clone(),
			Body:        request.Body,
		}
		if err := req.applySettings(restOptions.Settings, restOptions.Explain); err != nil {
			return nil, err
		}
		if len(buf) > 0 {
			req.Body = bytes.NewReader(buf)
		}
		requests = append(requests, req)
	}
	return requests, nil
}

func (req *RetryableRequest) getServerConfig(settings *rest.NDCRestSettings) *rest.ServerConfig {
	if settings == nil {
		return nil
	}
	if req.ServerID == "" {
		return &settings.Servers[0]
	}
	for _, server := range settings.Servers {
		if server.ID == req.ServerID {
			return &server
		}
	}

	return nil
}

func (req *RetryableRequest) applySecurity(serverConfig *rest.ServerConfig, isExplain bool) error {
	if serverConfig == nil {
		return nil
	}

	securitySchemes := serverConfig.SecuritySchemes
	securities := req.RawRequest.Security
	if req.RawRequest.Security.IsEmpty() && serverConfig.Security != nil {
		securities = serverConfig.Security
	}

	if securities.IsOptional() || len(securitySchemes) == 0 {
		return nil
	}

	var securityScheme *rest.SecurityScheme
	for _, security := range securities {
		sc, ok := securitySchemes[security.Name()]
		if !ok {
			continue
		}

		securityScheme = &sc
		if slices.Contains([]rest.SecuritySchemeType{rest.HTTPAuthScheme, rest.APIKeyScheme}, sc.Type) &&
			sc.Value != nil && sc.GetValue() != "" {
			break
		}
	}

	if securityScheme == nil {
		return nil
	}

	if req.Headers == nil {
		req.Headers = http.Header{}
	}

	switch securityScheme.Type {
	case rest.HTTPAuthScheme:
		headerName := securityScheme.Header
		if headerName == "" {
			headerName = "Authorization"
		}
		scheme := securityScheme.Scheme
		if scheme == "bearer" || scheme == "basic" {
			scheme = utils.ToPascalCase(securityScheme.Scheme)
		}
		v := securityScheme.GetValue()
		if v != "" {
			req.Headers.Set(headerName, fmt.Sprintf("%s %s", scheme, eitherMaskSecret(v, isExplain)))
		}
	case rest.APIKeyScheme:
		switch securityScheme.In {
		case rest.APIKeyInHeader:
			if securityScheme.Value != nil {
				value := securityScheme.GetValue()
				if value != "" {
					req.Headers.Set(securityScheme.Name, eitherMaskSecret(value, isExplain))
				}
			}
		case rest.APIKeyInQuery:
			value := securityScheme.GetValue()
			if value != "" {
				endpoint, err := url.Parse(req.URL)
				if err != nil {
					return err
				}

				q := endpoint.Query()
				q.Add(securityScheme.Name, eitherMaskSecret(value, isExplain))
				endpoint.RawQuery = q.Encode()
				req.URL = endpoint.String()
			}
		case rest.APIKeyInCookie:
			// Cookie header should be forwarded from Hasura engine
		default:
			return fmt.Errorf("unsupported location for apiKey scheme: %s", securityScheme.In)
		}
	// TODO: support OAuth and OIDC
	// Authentication headers can be forwarded from Hasura engine
	case rest.OAuth2Scheme, rest.OpenIDConnectScheme:
	default:
		return fmt.Errorf("unsupported security scheme: %s", securityScheme.Type)
	}
	return nil
}

func (req *RetryableRequest) applySettings(settings *rest.NDCRestSettings, isExplain bool) error {
	if settings == nil {
		return nil
	}
	serverConfig := req.getServerConfig(settings)
	if serverConfig == nil {
		return nil
	}
	if err := req.applySecurity(serverConfig, isExplain); err != nil {
		return err
	}

	req.applyDefaultHeaders(serverConfig.GetHeaders())
	req.applyDefaultHeaders(settings.GetHeaders())

	return nil
}

func (req *RetryableRequest) applyDefaultHeaders(defaultHeaders map[string]string) {
	for k, envValue := range defaultHeaders {
		if req.Headers.Get(k) != "" {
			continue
		}
		if envValue != "" {
			req.Headers.Set(k, envValue)
		}
	}
}
