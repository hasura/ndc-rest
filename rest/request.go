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

func (c *RESTConnector) createRequest(ctx context.Context, rawRequest *rest.Request, headers http.Header, data any) (*http.Request, context.CancelFunc, error) {
	var body io.Reader
	logger := connector.GetLogger(ctx)
	contentType := contentTypeJSON

	logAttrs := []any{
		slog.String("request_url", rawRequest.URL),
		slog.String("request_method", rawRequest.Method),
	}

	if rawRequest.RequestBody != nil {
		contentType = rawRequest.RequestBody.ContentType
		if data != nil {
			var buffer *bytes.Buffer
			if strings.HasPrefix(contentType, "text/") {
				buffer = bytes.NewBuffer([]byte(fmt.Sprintln(data)))
			} else if strings.HasPrefix(contentType, "multipart/") {
				buffer = new(bytes.Buffer)
				writer := internal.NewMultipartWriter(buffer)
				dataMap, ok := data.(map[string]any)
				if !ok {
					return nil, nil, fmt.Errorf("failed to decode request body, expect object, got: %v", data)
				}
				if rawRequest.RequestBody.Schema.Type != "object" || len(rawRequest.RequestBody.Schema.Properties) == 0 {
					return nil, nil, errors.New("invalid object schema for multipart")
				}

				for key := range dataMap {
					prop, ok := rawRequest.RequestBody.Schema.Properties[key]
					if !ok {
						continue
					}
					var enc *rest.EncodingObject
					if len(rawRequest.RequestBody.Encoding) > 0 {
						en, ok := rawRequest.RequestBody.Encoding[key]
						if ok {
							enc = &en
						}
					}
					err := c.evalMultipartFieldValue(writer, dataMap, key, &prop, enc, []string{key})
					if err != nil {
						return nil, nil, err
					}
				}
				if err := writer.Close(); err != nil {
					return nil, nil, err
				}
				contentType = writer.FormDataContentType()
			} else {
				switch contentType {
				case rest.ContentTypeFormURLEncoded:
					// do nothing, body properties are moved to parameters
				case rest.ContentTypeJSON:
					bodyBytes, err := json.Marshal(data)
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
			slog.Any("raw_request_body", data),
		)
		logger.Debug("sending request to remote server...", logAttrs...)
	}

	return request, cancel, nil
}

func (c *RESTConnector) evalMultipartFieldValue(w *internal.MultipartWriter, arguments map[string]any, name string, typeSchema *rest.TypeSchema, encObject *rest.EncodingObject, fieldPaths []string) error {

	value, ok := arguments[name]
	if !ok || utils.IsNil(value) {
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

	// if slices.Contains([]string{"object", "array"}, typeSchema.Type) && (encObject == nil || slices.Contains(encObject.ContentType, rest.ContentTypeJSON)) {
	if slices.Contains([]string{"object", "array"}, typeSchema.Type) {
		return w.WriteJSON(name, value, headers)
	}

	switch typeSchema.Type {
	case "file", string(rest.ScalarBinary):
		return w.WriteDataURI(name, value, headers)
	default:
		params, err := c.encodeParameterValues(typeSchema, value, fieldPaths)
		if err != nil {
			return err
		}

		if len(params) == 0 {
			return nil
		}

		return w.WriteField(name, params.String(), headers)
	}
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

		headerParams, err := c.encodeParameterValues(param.Schema, rawHeaderValue, []string{key})
		if err != nil {
			return nil, err
		}

		setHeaderParameters(&results, &param, headerParams)
	}

	return results, nil
}
