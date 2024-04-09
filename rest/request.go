package rest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"slices"
	"strings"
	"time"

	rest "github.com/hasura/ndc-rest-schema/schema"
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
		// TODO: validate data with request body
		if data != nil {
			var buffer *bytes.Buffer
			if strings.HasPrefix(contentType, "text/") {
				buffer = bytes.NewBuffer([]byte(fmt.Sprintln(data)))
			} else if strings.HasPrefix(contentType, "multipart/") {
				buffer = new(bytes.Buffer)
				writer := multipart.NewWriter(buffer)
				dataMap, ok := data.(map[string]any)
				if !ok {
					return nil, nil, fmt.Errorf("failed to decode request body, expect object, got: %v", data)
				}
				if rawRequest.RequestBody.Schema.Type != "object" || len(rawRequest.RequestBody.Schema.Properties) == 0 {
					return nil, nil, errors.New("invalid object schema for multipart")
				}

				for key, value := range dataMap {
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
					err := c.evalMultipartFieldValue(writer, key, &prop, enc, value, []string{key})
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

func (c *RESTConnector) evalMultipartFieldValue(w *multipart.Writer, name string, typeSchema *rest.TypeSchema, encObject *rest.EncodingObject, value any, fieldPaths []string) error {

	if utils.IsNil(value) {
		return nil
	}

	if slices.Contains([]string{"object", "array"}, typeSchema.Type) && (encObject == nil || slices.Contains(encObject.ContentType, rest.ContentTypeJSON)) {
		return writeMultipartFieldJSON(w, name, value)
	}

	switch typeSchema.Type {
	case "file", string(rest.ScalarBinary):
		b64, err := utils.DecodeString(value)
		if err != nil {
			return fmt.Errorf("%s: %s", name, err)
		}
		rawDecodedBytes, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return fmt.Errorf("%s: %s", name, err)
		}
		p, err := w.CreateFormFile(name, name)
		if err != nil {
			return fmt.Errorf("%s: %s", name, err)
		}
		_, err = p.Write(rawDecodedBytes)
		return err
	default:
		params, err := c.encodeParameterValues(typeSchema, value, fieldPaths)
		if err != nil {
			return err
		}

		if len(params) == 0 {
			return nil
		}

		return w.WriteField(name, params.String())
	}
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

func writeMultipartFieldJSON(w *multipart.Writer, fieldName string, value any) error {
	bs, err := json.Marshal(value)
	if err != nil {
		return err
	}

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"`, escapeQuotes(fieldName)))
	h.Set(rest.ContentTypeHeader, rest.ContentTypeJSON)
	p, err := w.CreatePart(h)
	if err != nil {
		return err
	}

	_, err = p.Write(bs)
	return err
}
