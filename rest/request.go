package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/rest/internal"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/utils"
)

func (c *RESTConnector) createRequest(ctx context.Context, rawRequest *rest.Request, headers http.Header, arguments map[string]any) (*http.Request, context.CancelFunc, error) {
	var body io.Reader
	logger := connector.GetLogger(ctx)
	contentType := contentTypeJSON

	logAttrs := []any{
		slog.String("request_url", rawRequest.URL),
		slog.String("request_method", rawRequest.Method),
	}

	bodyData, ok := arguments["body"]
	if rawRequest.RequestBody != nil {
		contentType = rawRequest.RequestBody.ContentType
		if ok && bodyData != nil {
			var buffer *bytes.Buffer
			if strings.HasPrefix(contentType, "text/") {
				buffer = bytes.NewBuffer([]byte(fmt.Sprintln(bodyData)))
			} else if strings.HasPrefix(contentType, "multipart/") {
				var err error
				buffer, contentType, err = c.createMultipartForm(rawRequest.RequestBody, arguments)
				if err != nil {
					return nil, nil, err
				}
			} else {
				switch contentType {
				case rest.ContentTypeFormURLEncoded:
					// do nothing, body properties are moved to parameters
				case rest.ContentTypeJSON:
					bodyBytes, err := json.Marshal(bodyData)
					if err != nil {
						return nil, nil, err
					}

					buffer = bytes.NewBuffer(bodyBytes)
				default:
					return nil, nil, fmt.Errorf("unsupported content type %s", contentType)
				}
			}

			if logger.Enabled(ctx, slog.LevelDebug) {
				logAttrs = append(logAttrs,
					slog.Int("content_length", buffer.Len()),
					slog.String("body", string(buffer.String())),
				)
			}

			body = buffer
		} else if contentType != rest.ContentTypeFormURLEncoded &&
			(rawRequest.RequestBody.Schema != nil && !rawRequest.RequestBody.Schema.Nullable) {
			return nil, nil, errors.New("request body is required")
		}
	}

	timeout := defaultTimeout
	if rawRequest.Timeout > 0 {
		timeout = rawRequest.Timeout
	}

	ctxR, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	request, err := http.NewRequestWithContext(ctxR, strings.ToUpper(rawRequest.Method), rawRequest.URL, body)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	for key, header := range headers {
		request.Header[key] = header
	}
	request.Header.Set(rest.ContentTypeHeader, contentType)

	if logger.Enabled(ctx, slog.LevelDebug) {
		logAttrs = append(logAttrs,
			slog.Any("request_headers", request.Header),
			slog.Any("raw_request_body", bodyData),
		)
		logger.Debug("sending request to remote server...", logAttrs...)
	}

	return request, cancel, nil
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
			if len(values) == 1 {
				if err = w.WriteField(fieldName, values[0], headers); err != nil {
					return err
				}
			} else if len(values) > 1 {
				fieldName = fmt.Sprintf("%s[]", fieldName)
				for _, v := range values {
					if err = w.WriteField(fieldName, v, headers); err != nil {
						return err
					}
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
