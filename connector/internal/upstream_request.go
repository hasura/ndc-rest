package internal

import (
	"fmt"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// RequestBuilderResults hold the result of built requests.
type RequestBuilderResults struct {
	Requests  []*RetryableRequest
	Operation *rest.OperationInfo
	Schema    *configuration.NDCHttpRuntimeSchema

	*HTTPOptions
}

func (um *UpstreamManager) BuildRequests(runtimeSchema *configuration.NDCHttpRuntimeSchema, operationName string, operation *rest.OperationInfo, rawArgs map[string]any) (*RequestBuilderResults, error) {
	// 1. parse http options from arguments
	httpOptions, err := um.parseHTTPOptionsFromArguments(operation.Arguments, rawArgs)
	if err != nil {
		return nil, schema.UnprocessableContentError("invalid http options", map[string]any{
			"cause": err.Error(),
		})
	}

	upstream, ok := um.upstreams[runtimeSchema.Name]
	if !ok {
		return nil, schema.InternalServerError(fmt.Sprintf("upstream with namespace %s does not exist", runtimeSchema.Name), nil)
	}

	if len(upstream.servers) == 0 {
		return nil, schema.InternalServerError("no available server in the upstream with namespace "+runtimeSchema.Name, nil)
	}

	// 2. get headers in argument if exists
	headers, err := um.getArgumentHeaders(rawArgs)
	if err != nil {
		return nil, err
	}

	// 3. apply argument presets if exists
	if upstream.argumentPresets != nil {
		rawArgs, err = upstream.argumentPresets.Apply(operationName, rawArgs, headers)
		if err != nil {
			return nil, err
		}
	}

	results := &RequestBuilderResults{
		Operation:   operation,
		Schema:      runtimeSchema,
		HTTPOptions: httpOptions,
	}
	results.HTTPOptions.Concurrency = um.config.Concurrency.HTTP

	if strings.HasPrefix(operation.Request.URL, "http") {
		// 4. build the request
		req, err := NewRequestBuilder(runtimeSchema.NDCHttpSchema, operation, rawArgs, runtimeSchema.Runtime).Build()
		if err != nil {
			return nil, err
		}
		req.Namespace = runtimeSchema.Name

		if err := evalForwardedHeaders(req, headers); err != nil {
			return nil, schema.UnprocessableContentError("invalid forwarded headers", map[string]any{
				"cause": err.Error(),
			})
		}

		results.Requests = []*RetryableRequest{req}

		return results, nil
	}

	if !httpOptions.Distributed || len(upstream.servers) == 1 {
		req, err := upstream.buildRequest(runtimeSchema, operationName, operation, rawArgs, headers, httpOptions.Servers)
		if err != nil {
			return nil, err
		}
		results.Requests = []*RetryableRequest{req}

		return results, nil
	}

	serverIDs := httpOptions.Servers
	if len(serverIDs) == 0 {
		serverIDs = utils.GetKeys(upstream.servers)
	}

	for _, serverID := range serverIDs {
		req, err := upstream.buildRequest(runtimeSchema, operationName, operation, rawArgs, headers, []string{serverID})
		if err != nil {
			return nil, err
		}
		results.Requests = append(results.Requests, req)
	}

	return results, nil
}

func (um *UpstreamManager) parseHTTPOptionsFromArguments(argumentsInfo map[string]rest.ArgumentInfo, rawArgs map[string]any) (*HTTPOptions, error) {
	var result HTTPOptions
	argInfo, ok := argumentsInfo[rest.HTTPOptionsArgumentName]
	if !ok {
		return &result, nil
	}
	rawHttpOptions, ok := rawArgs[rest.HTTPOptionsArgumentName]
	if ok {
		if err := result.FromValue(rawHttpOptions); err != nil {
			return nil, err
		}
	}
	httpOptionsNamedType := schema.GetUnderlyingNamedType(argInfo.Type)
	result.Distributed = httpOptionsNamedType != nil && httpOptionsNamedType.Name == rest.HTTPDistributedOptionsObjectName

	return &result, nil
}

func (um *UpstreamManager) getArgumentHeaders(rawArgs map[string]any) (map[string]string, error) {
	headers := make(map[string]string)
	if !um.config.ForwardHeaders.Enabled || um.config.ForwardHeaders.ArgumentField == nil || *um.config.ForwardHeaders.ArgumentField == "" {
		return headers, nil
	}
	rawHeaders, ok := rawArgs[*um.config.ForwardHeaders.ArgumentField]
	if !ok {
		return headers, nil
	}

	if err := mapstructure.Decode(rawHeaders, &headers); err != nil {
		return nil, schema.UnprocessableContentError(fmt.Sprintf("arguments.%s: %s", *um.config.ForwardHeaders.ArgumentField, err), nil)
	}

	return headers, nil
}
