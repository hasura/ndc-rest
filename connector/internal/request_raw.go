package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/hasura/ndc-http/connector/internal/contenttype"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	restUtils "github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
)

// RawRequestBuilder represents a type to build a raw HTTP request
type RawRequestBuilder struct {
	operation      schema.MutationOperation
	forwardHeaders configuration.ForwardHeadersSettings
}

// NewRawRequestBuilder create a new RawRequestBuilder instance.
func NewRawRequestBuilder(operation schema.MutationOperation, forwardHeaders configuration.ForwardHeadersSettings) *RawRequestBuilder {
	return &RawRequestBuilder{
		operation:      operation,
		forwardHeaders: forwardHeaders,
	}
}

func (rqe *RawRequestBuilder) Explain() (*schema.ExplainResponse, error) {
	httpRequest, err := rqe.explain()
	if err != nil {
		return nil, err
	}

	explainResp := &schema.ExplainResponse{
		Details: schema.ExplainResponseDetails{},
	}

	if httpRequest.Body != nil {
		bodyBytes, err := io.ReadAll(httpRequest.Body)
		if err != nil {
			return nil, schema.InternalServerError("failed to read request body", map[string]any{
				"cause": err.Error(),
			})
		}
		httpRequest.Body = nil
		explainResp.Details["body"] = string(bodyBytes)
	}

	// mask sensitive forwarded headers if exists
	for key, value := range httpRequest.Headers {
		if IsSensitiveHeader(key) {
			httpRequest.Headers.Set(key, restUtils.MaskString(value[0]))
		}
	}

	explainResp.Details["url"] = httpRequest.URL.String()
	rawHeaders, err := json.Marshal(httpRequest.Headers)
	if err != nil {
		return nil, schema.InternalServerError("failed to encode headers", map[string]any{
			"cause": err.Error(),
		})
	}
	explainResp.Details["headers"] = string(rawHeaders)

	return explainResp, nil
}

// Build evaluates and builds the raw request.
func (rqe *RawRequestBuilder) Build() (*RequestBuilderResults, error) {
	httpRequest, err := rqe.explain()
	if err != nil {
		return nil, err
	}

	return &RequestBuilderResults{
		Requests:    []*RetryableRequest{httpRequest},
		HTTPOptions: &HTTPOptions{},
		Schema:      &configuration.NDCHttpRuntimeSchema{},
	}, nil
}

func (rqe *RawRequestBuilder) explain() (*RetryableRequest, error) {
	request, err := rqe.decodeArguments()
	if err != nil {
		return nil, schema.UnprocessableContentError(err.Error(), nil)
	}

	return request, nil
}

func (rqe *RawRequestBuilder) decodeArguments() (*RetryableRequest, error) {
	var rawArguments map[string]json.RawMessage
	if err := json.Unmarshal(rqe.operation.Arguments, &rawArguments); err != nil {
		return nil, err
	}

	rawURL, ok := rawArguments["url"]
	if !ok || len(rawURL) == 0 {
		return nil, errors.New("url is required")
	}

	var urlString string
	if err := json.Unmarshal(rawURL, &urlString); err != nil {
		return nil, fmt.Errorf("url: %w", err)
	}
	requestURL, err := rest.ParseHttpURL(urlString)
	if err != nil {
		return nil, fmt.Errorf("url: %w", err)
	}

	rawMethod, ok := rawArguments["method"]
	if !ok || len(rawMethod) == 0 {
		return nil, errors.New("method is required")
	}

	var method string
	if err := json.Unmarshal(rawMethod, &method); err != nil {
		return nil, fmt.Errorf("method: %w", err)
	}

	if !slices.Contains(httpMethod_enums, method) {
		return nil, fmt.Errorf("invalid http method, expected %v, got %s", httpMethod_enums, method)
	}

	var timeout int
	if rawTimeout, ok := rawArguments["timeout"]; ok {
		if err := json.Unmarshal(rawTimeout, &timeout); err != nil {
			return nil, fmt.Errorf("timeout: %w", err)
		}

		if timeout < 0 {
			return nil, errors.New("timeout must not be negative")
		}
	}

	var retryPolicy rest.RetryPolicy
	if rawRetry, ok := rawArguments["retry"]; ok {
		if err := json.Unmarshal(rawRetry, &retryPolicy); err != nil {
			return nil, fmt.Errorf("retry: %w", err)
		}
	}

	headers := http.Header{}
	contentType := rest.ContentTypeJSON
	if rqe.forwardHeaders.Enabled && rqe.forwardHeaders.ArgumentField != nil && *rqe.forwardHeaders.ArgumentField != "" {
		if rawHeaders, ok := rawArguments[*rqe.forwardHeaders.ArgumentField]; ok {
			var fwHeaders map[string]string
			if err := json.Unmarshal(rawHeaders, &fwHeaders); err != nil {
				return nil, fmt.Errorf("%s: %w", *rqe.forwardHeaders.ArgumentField, err)
			}

			for key, value := range fwHeaders {
				headers.Set(key, value)
			}
		}
	}

	if rawHeaders, ok := rawArguments["additionalHeaders"]; ok {
		var additionalHeaders map[string]string
		if err := json.Unmarshal(rawHeaders, &additionalHeaders); err != nil {
			return nil, fmt.Errorf("additionalHeaders: %w", err)
		}

		for key, value := range additionalHeaders {
			if strings.ToLower(key) == "content-type" && value != "" {
				contentType = value
			}
			headers.Set(key, value)
		}
	}

	request := &RetryableRequest{
		URL:         *requestURL,
		Headers:     headers,
		ContentType: contentType,
		RawRequest: &rest.Request{
			URL:    urlString,
			Method: method,
		},
		Runtime: rest.RuntimeSettings{
			Timeout: uint(timeout),
			Retry:   retryPolicy,
		},
	}

	if method == "get" || method == "delete" {
		return request, nil
	}

	if rawBody, ok := rawArguments["body"]; ok && len(rawBody) > 0 {
		reader, contentType, contentLength, err := rqe.evalRequestBody(rawBody, contentType)
		if err != nil {
			return nil, fmt.Errorf("body: %w", err)
		}
		request.ContentType = contentType
		request.ContentLength = contentLength
		request.Body = reader
	}

	return request, nil
}

func (rqe *RawRequestBuilder) evalRequestBody(rawBody json.RawMessage, contentType string) (io.ReadSeeker, string, int64, error) {
	switch {
	case restUtils.IsContentTypeJSON(contentType):
		if !json.Valid(rawBody) {
			return nil, "", 0, fmt.Errorf("invalid json body: %s", string(rawBody))
		}

		return bytes.NewReader(rawBody), contentType, int64(len(rawBody)), nil
	case restUtils.IsContentTypeXML(contentType):
		var bodyData any
		if err := json.Unmarshal(rawBody, &bodyData); err != nil {
			return nil, "", 0, fmt.Errorf("invalid body: %w", err)
		}

		if bodyStr, ok := bodyData.(string); ok {
			return strings.NewReader(bodyStr), contentType, int64(len(bodyStr)), nil
		}

		bodyBytes, err := contenttype.NewXMLEncoder(nil).EncodeArbitrary(bodyData)
		if err != nil {
			return nil, "", 0, err
		}

		return bytes.NewReader(bodyBytes), contentType, int64(len(bodyBytes)), nil
	case restUtils.IsContentTypeText(contentType):
		var bodyData string
		if err := json.Unmarshal(rawBody, &bodyData); err != nil {
			return nil, "", 0, fmt.Errorf("invalid body: %w", err)
		}

		return strings.NewReader(bodyData), contentType, int64(len(bodyData)), nil
	case restUtils.IsContentTypeMultipartForm(contentType):
		var bodyData any
		if err := json.Unmarshal(rawBody, &bodyData); err != nil {
			return nil, "", 0, fmt.Errorf("invalid body: %w", err)
		}
		r, contentType, err := contenttype.NewMultipartFormEncoder(nil, nil, nil).EncodeArbitrary(bodyData)
		if err != nil {
			return nil, "", 0, err
		}

		return r, contentType, r.Size(), nil
	case contentType == rest.ContentTypeFormURLEncoded:
		var bodyData any
		if err := json.Unmarshal(rawBody, &bodyData); err != nil {
			return nil, "", 0, fmt.Errorf("invalid body: %w", err)
		}

		if bodyStr, ok := bodyData.(string); ok {
			return strings.NewReader(bodyStr), contentType, int64(len(bodyStr)), nil
		}

		r, size, err := contenttype.NewURLParameterEncoder(nil, rest.ContentTypeFormURLEncoded).EncodeArbitrary(bodyData)

		return r, contentType, size, err
	default:
		var bodyData string
		if err := json.Unmarshal(rawBody, &bodyData); err != nil {
			return nil, "", 0, fmt.Errorf("invalid body: %w", err)
		}
		dataURI, err := contenttype.DecodeDataURI(bodyData)
		if err != nil {
			return nil, "", 0, err
		}
		r := bytes.NewReader([]byte(dataURI.Data))

		return r, contentType, r.Size(), nil
	}
}
