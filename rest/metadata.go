package rest

import (
	"fmt"
	"net/url"
	"strings"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
)

// RESTMetadataCollection stores list of REST metadata with helper methods
type RESTMetadataCollection []RESTMetadata

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

	return rm.applySecurity(req)
}

func (rm RESTMetadata) buildURL(endpoint string) string {
	if strings.HasPrefix(endpoint, "http") {
		return endpoint
	}
	var host string
	for _, server := range rm.settings.Servers {
		host = server.URL
		break
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

	security := req.Security.First()
	securityScheme, ok := rm.settings.SecuritySchemes[security.Name()]
	if !ok {
		return req, nil
	}

	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	switch securityScheme.Type {
	case rest.HTTPAuthScheme:
		headerName := securityScheme.Header
		if headerName == "" {
			headerName = "Authorization"
		}
		scheme := securityScheme.Name
		if securityScheme.Name == "bearer" || securityScheme.Name == "basic" {
			scheme = utils.ToPascalCase(securityScheme.Name)
		}
		req.Headers[headerName] = fmt.Sprintf("%s %s", scheme, securityScheme.Value)
	case rest.APIKeyScheme:
		switch securityScheme.In {
		case rest.APIKeyInHeader:
			req.Headers[securityScheme.Name] = securityScheme.Value
		case rest.APIKeyInQuery:
			endpoint, err := url.Parse(req.URL)
			if err != nil {
				return nil, err
			}

			q := endpoint.Query()
			q.Add(securityScheme.Name, securityScheme.Value)
			endpoint.RawQuery = q.Encode()
			req.URL = endpoint.String()
		case rest.APIKeyInCookie:
			req.Headers["Cookie"] = fmt.Sprintf("%s=%s", securityScheme.Name, securityScheme.Value)
		default:
			return nil, fmt.Errorf("unsupported location for apiKey scheme: %s", securityScheme.In)
		}
	// TODO: support OAuth and OIDC
	default:
		return nil, fmt.Errorf("unsupported security scheme: %s", securityScheme.Type)
	}

	return req, nil
}
