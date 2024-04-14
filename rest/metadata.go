package rest

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
)

// RESTMetadataCollection stores list of REST metadata with helper methods
type RESTMetadataCollection []RESTMetadata

// GetFunction gets the NDC function by name
func (rms RESTMetadataCollection) GetFunction(name string) (*rest.RESTFunctionInfo, error) {
	for _, rm := range rms {
		fn, err := rm.GetFunction(name)
		if err != nil {
			return nil, err
		}
		if fn != nil {
			return fn, nil
		}
	}
	return nil, schema.UnprocessableContentError(fmt.Sprintf("unsupported query: %s", name), nil)
}

// GetProcedure gets the NDC procedure by name
func (rms RESTMetadataCollection) GetProcedure(name string) (*rest.RESTProcedureInfo, error) {
	for _, rm := range rms {
		fn, err := rm.GetProcedure(name)
		if err != nil {
			return nil, err
		}
		if fn != nil {
			return fn, nil
		}
	}
	return nil, schema.UnprocessableContentError(fmt.Sprintf("unsupported query: %s", name), nil)
}

// RESTMetadata stores REST schema with handy methods to build requests
type RESTMetadata struct {
	settings   *rest.NDCRestSettings
	functions  map[string]rest.RESTFunctionInfo
	procedures map[string]rest.RESTProcedureInfo
}

// GetFunction gets the NDC function by name
func (rm RESTMetadata) GetFunction(name string) (*rest.RESTFunctionInfo, error) {
	fn, ok := rm.functions[name]
	if !ok {
		return nil, nil
	}

	req, err := rm.buildRequest(fn.Request)
	if err != nil {
		return nil, err
	}
	return &rest.RESTFunctionInfo{
		Request:      req,
		FunctionInfo: fn.FunctionInfo,
	}, nil
}

// GetProcedure gets the NDC procedure by name
func (rm RESTMetadata) GetProcedure(name string) (*rest.RESTProcedureInfo, error) {
	fn, ok := rm.procedures[name]
	if !ok {
		return nil, nil
	}

	req, err := rm.buildRequest(fn.Request)
	if err != nil {
		return nil, err
	}
	return &rest.RESTProcedureInfo{
		Request:       req,
		ProcedureInfo: fn.ProcedureInfo,
	}, nil
}

func (rm RESTMetadata) buildRequest(rawReq *rest.Request) (*rest.Request, error) {
	req := rawReq.Clone()
	req.URL = rm.buildURL(req.URL)

	if req.Timeout == 0 {
		if rm.settings != nil && rm.settings.Timeout != nil {
			timeout, err := rm.settings.Timeout.Value()
			if err != nil {
				return nil, err
			}
			if timeout != nil && *timeout > 0 {
				req.Timeout = uint(*timeout)
			}
		}

		if req.Timeout == 0 {
			req.Timeout = defaultTimeoutSeconds
		}
	}
	if req.Retry == nil {
		req.Retry = &rest.RetryPolicy{}
	}

	if rm.settings.Retry != nil {
		if req.Retry.Times == 0 {
			times, err := rm.settings.Retry.Times.Value()
			if err == nil && times != nil && *times > 0 {
				req.Retry.Times = uint(*times)
			}
		}
		if req.Retry.Delay == 0 {
			delay, err := rm.settings.Retry.Delay.Value()
			if err == nil && delay != nil && *delay > 0 {
				req.Retry.Delay = uint(*delay)
			} else {
				req.Retry.Delay = defaultRetryDelays
			}
		}
		if len(req.Retry.HTTPStatus) == 0 {
			status, err := rm.settings.Retry.HTTPStatus.Value()
			if err != nil || len(status) == 0 {
				status = defaultRetryHTTPStatus
			}
			for _, st := range status {
				req.Retry.HTTPStatus = append(req.Retry.HTTPStatus, int(st))
			}
		}
	}

	return rm.applySecurity(req)
}

func (rm RESTMetadata) buildURL(endpoint string) string {
	if strings.HasPrefix(endpoint, "http") {
		return endpoint
	}
	var host string
	for _, server := range rm.settings.Servers {
		hostPtr := server.URL.Value()
		if hostPtr != nil {
			host = *hostPtr
			break
		}
	}

	return fmt.Sprintf("%s%s", host, endpoint)
}

func (rm RESTMetadata) applySecurity(req *rest.Request) (*rest.Request, error) {
	if req.Security.IsEmpty() {
		req.Security = rm.settings.Security
	}
	if req.Security.IsOptional() || len(rm.settings.SecuritySchemes) == 0 {
		return req, nil
	}

	var securityScheme *rest.SecurityScheme
	for _, security := range req.Security {
		sc, ok := rm.settings.SecuritySchemes[security.Name()]
		if !ok || (slices.Contains([]rest.SecuritySchemeType{rest.HTTPAuthScheme, rest.APIKeyScheme}, sc.Type) &&
			(sc.Value.Value() == nil || *sc.Value.Value() == "")) {
			continue
		}
		securityScheme = &sc
	}

	if securityScheme == nil {
		return req, nil
	}

	if req.Headers == nil {
		req.Headers = make(map[string]rest.EnvString)
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
		if securityScheme.Value != nil {
			v := securityScheme.Value.Value()
			if v != nil {
				req.Headers[headerName] = *rest.NewEnvStringValue(fmt.Sprintf("%s %s", scheme, *v))
			}
		}
	case rest.APIKeyScheme:
		switch securityScheme.In {
		case rest.APIKeyInHeader:
			if securityScheme.Value != nil {
				req.Headers[securityScheme.Name] = *securityScheme.Value
			}
		case rest.APIKeyInQuery:
			endpoint, err := url.Parse(req.URL)
			if err != nil {
				return nil, err
			}

			q := endpoint.Query()
			q.Add(securityScheme.Name, *securityScheme.Value.Value())
			endpoint.RawQuery = q.Encode()
			req.URL = endpoint.String()
		case rest.APIKeyInCookie:
			if securityScheme.Value != nil {
				v := securityScheme.Value.Value()
				if v != nil {
					req.Headers["Cookie"] = *rest.NewEnvStringValue(fmt.Sprintf("%s=%s", securityScheme.Name, *v))
				}
			}
		default:
			return nil, fmt.Errorf("unsupported location for apiKey scheme: %s", securityScheme.In)
		}
	// TODO: support OAuth and OIDC
	default:
		return nil, fmt.Errorf("unsupported security scheme: %s", securityScheme.Type)
	}

	return req, nil
}
