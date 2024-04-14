package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/rest/internal"
	"github.com/hasura/ndc-sdk-go/utils"
)

// RetryableRequest wraps the raw request with retryable
type RetryableRequest struct {
	rawRequest  *rest.Request
	contentType string
	headers     http.Header
	body        *bytes.Buffer
}

// CreateRequest creates an HTTP request with body copied
func (r *RetryableRequest) CreateRequest(ctx context.Context) (*http.Request, context.CancelFunc, error) {
	var body io.Reader
	if r.body != nil {
		body = r.body
	}
	ctxR, cancel := context.WithTimeout(ctx, time.Duration(r.rawRequest.Timeout)*time.Second)
	request, err := http.NewRequestWithContext(ctxR, strings.ToUpper(r.rawRequest.Method), r.rawRequest.URL, body)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	for key, header := range r.headers {
		request.Header[key] = header
	}
	request.Header.Set(rest.ContentTypeHeader, r.contentType)

	return request, cancel, nil
}

func (c *RESTConnector) createRequest(rawRequest *rest.Request, headers http.Header, arguments map[string]any) (*RetryableRequest, error) {
	var buffer *bytes.Buffer
	contentType := contentTypeJSON

	bodyData, ok := arguments["body"]
	if rawRequest.RequestBody != nil {
		contentType = rawRequest.RequestBody.ContentType
		if ok && bodyData != nil {
			binaryBody := getRequestUploadBody(rawRequest)
			if binaryBody != nil {
				b64, err := utils.DecodeString(bodyData)
				if err != nil {
					return nil, err
				}
				dataURI, err := internal.DecodeDataURI(b64)
				if err != nil {
					return nil, err
				}
				buffer = bytes.NewBuffer([]byte(dataURI.Data))
			} else if strings.HasPrefix(contentType, "text/") {
				buffer = bytes.NewBuffer([]byte(fmt.Sprint(bodyData)))
			} else if strings.HasPrefix(contentType, "multipart/") {
				var err error
				buffer, contentType, err = c.createMultipartForm(rawRequest.RequestBody, arguments)
				if err != nil {
					return nil, err
				}
			} else {
				switch contentType {
				case rest.ContentTypeFormURLEncoded:
					// do nothing, body properties are moved to parameters
				case rest.ContentTypeJSON:
					bodyBytes, err := json.Marshal(bodyData)
					if err != nil {
						return nil, err
					}

					buffer = bytes.NewBuffer(bodyBytes)
				default:
					return nil, fmt.Errorf("unsupported content type %s", contentType)
				}
			}
		} else if contentType != rest.ContentTypeFormURLEncoded &&
			(rawRequest.RequestBody.Schema != nil && !rawRequest.RequestBody.Schema.Nullable) {
			return nil, errors.New("request body is required")
		}
	}

	request := &RetryableRequest{
		rawRequest:  rawRequest,
		contentType: contentType,
		headers:     headers,
		body:        buffer,
	}

	return request, nil
}

func (c *RESTConnector) createMultipartForm(reqBody *rest.RequestBody, arguments map[string]any) (*bytes.Buffer, string, error) {
	bodyData := arguments["body"]

	buffer := new(bytes.Buffer)
	writer := internal.NewMultipartWriter(buffer)
	dataMap, ok := bodyData.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("failed to decode request body, expect object, got: %v", bodyData)
	}
	if reqBody.Schema.Type != "object" || len(reqBody.Schema.Properties) == 0 {
		return nil, "", errors.New("invalid object schema for multipart")
	}

	for key, value := range dataMap {
		prop, ok := reqBody.Schema.Properties[key]
		if !ok {
			continue
		}
		var enc *rest.EncodingObject
		if len(reqBody.Encoding) > 0 {
			en, ok := reqBody.Encoding[key]
			if ok {
				enc = &en
			}
		}
		err := c.evalMultipartFieldValue(writer, arguments, key, value, &prop, enc)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %s", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	return buffer, writer.FormDataContentType(), nil
}

func (c *RESTConnector) evalMultipartFieldValue(w *internal.MultipartWriter, arguments map[string]any, name string, value any, typeSchema *rest.TypeSchema, encObject *rest.EncodingObject) error {
	if utils.IsNil(value) {
		return nil
	}

	var headers http.Header
	var err error
	if encObject != nil && len(encObject.Headers) > 0 {
		headers, err = c.evalEncodingHeaders(encObject.Headers, arguments)
		if err != nil {
			return err
		}
	}

	if slices.Contains([]string{"object", "array"}, typeSchema.Type) && (encObject == nil || slices.Contains(encObject.ContentType, rest.ContentTypeJSON)) {
		return w.WriteJSON(name, value, headers)
	}

	switch typeSchema.Type {
	case "file", string(rest.ScalarBinary):
		return w.WriteDataURI(name, value, headers)
	default:
		params, err := c.encodeParameterValues(typeSchema, value, []string{})
		if err != nil {
			return err
		}

		if len(params) == 0 {
			return nil
		}

		for _, p := range params {
			keys := p.Keys()
			values := p.Values()
			fieldName := name

			if len(keys) > 0 {
				keys = append([]internal.Key{internal.NewKey(name)}, keys...)
				fieldName = keys.String()
			}

			if typeSchema.Type == "array" || len(values) > 1 {
				fieldName = fmt.Sprintf("%s[]", fieldName)
				for _, v := range values {
					if err = w.WriteField(fieldName, v, headers); err != nil {
						return err
					}
				}
			} else if len(values) == 1 {
				if err = w.WriteField(fieldName, values[0], headers); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (c *RESTConnector) evalEncodingHeaders(encHeaders map[string]rest.RequestParameter, arguments map[string]any) (http.Header, error) {
	results := http.Header{}
	for key, param := range encHeaders {
		argumentName := param.ArgumentName
		if argumentName == "" {
			argumentName = key
		}
		rawHeaderValue, ok := arguments[argumentName]
		if !ok {
			continue
		}

		headerParams, err := c.encodeParameterValues(param.Schema, rawHeaderValue, []string{})
		if err != nil {
			return nil, err
		}

		param.Name = key
		setHeaderParameters(&results, &param, headerParams)
	}

	return results, nil
}

func getRequestUploadBody(rawRequest *rest.Request) *rest.RequestBody {
	if rawRequest.RequestBody == nil {
		return nil
	}
	if rawRequest.RequestBody.ContentType == "application/octet-stream" {
		return rawRequest.RequestBody
	}
	if rawRequest.RequestBody.Schema != nil && rawRequest.RequestBody.Schema.Type == string(rest.ScalarBinary) {
		return rawRequest.RequestBody
	}
	return nil
}
