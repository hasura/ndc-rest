package internal

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"path"
	"slices"

	"github.com/hasura/ndc-http/connector/internal/argument"
	"github.com/hasura/ndc-http/connector/internal/security"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

// Server contains server settings.
type Server struct {
	URL             *url.URL
	Headers         map[string]string
	Credentials     map[string]security.Credential
	ArgumentPresets *argument.ArgumentPresets
	Security        rest.AuthSecurities
	HTTPClient      *http.Client
}

// UpstreamSetting represents a setting for upstream servers.
type UpstreamSetting struct {
	httpClient      *http.Client
	servers         map[string]Server
	headers         map[string]string
	security        rest.AuthSecurities
	credentials     map[string]security.Credential
	argumentPresets *argument.ArgumentPresets
}

func (us *UpstreamSetting) buildRequest(runtimeSchema *configuration.NDCHttpRuntimeSchema, operationName string, operation *rest.OperationInfo, arguments map[string]any, headers map[string]string, servers []string) (*RetryableRequest, error) {
	baseURL, serverID, err := us.getBaseURLFromServers(runtimeSchema.Name, servers)
	if err != nil {
		return nil, err
	}

	server := us.servers[serverID]
	if server.ArgumentPresets != nil {
		arguments, err = server.ArgumentPresets.Apply(operationName, arguments, headers)
		if err != nil {
			return nil, err
		}
	}

	req, err := NewRequestBuilder(runtimeSchema.NDCHttpSchema, operation, arguments, runtimeSchema.Runtime).Build()
	if err != nil {
		return nil, err
	}
	req.Namespace = runtimeSchema.Name

	if err := evalForwardedHeaders(req, headers); err != nil {
		return nil, schema.UnprocessableContentError("invalid forwarded headers", map[string]any{
			"cause": err.Error(),
		})
	}

	req.URL.Scheme = baseURL.Scheme
	req.URL.Host = baseURL.Host
	req.URL.Path = path.Join(baseURL.Path, req.URL.Path)
	req.ServerID = serverID

	return req, nil
}

func (us *UpstreamSetting) getBaseURLFromServers(namespace string, serverIDs []string) (*url.URL, string, error) {
	var results []*url.URL
	var selectedServerIDs []string
	for key, server := range us.servers {
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
