package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// Doer abstracts a HTTP client with Do method
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type httpClient struct {
	Client Doer
}

func createHTTPClient(client Doer) *httpClient {
	return &httpClient{
		Client: client,
	}
}

// Send creates and executes the request and evaluate response selection
func (client *httpClient) Send(ctx context.Context, request *http.Request, selection schema.NestedField) (any, error) {
	resp, err := client.Client.Do(request)
	if err != nil {
		return nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
	}

	logger := connector.GetLogger(ctx)
	if logger.Enabled(ctx, slog.LevelDebug) {
		logAttrs := []any{
			slog.Int("http_status", resp.StatusCode),
			slog.Any("response_headers", resp.Header),
		}

		if resp.Body != nil {
			respBody, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				return nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
			}
			logAttrs = append(logAttrs, slog.String("response_body", string(respBody)))
			resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
		}

		logger.Debug("received response from remote server", logAttrs...)
	}

	return evalHTTPResponse(resp, selection)
}

func evalHTTPResponse(resp *http.Response, selection schema.NestedField) (any, error) {
	contentType := parseContentType(resp.Header.Get(contentTypeHeader))
	if resp.StatusCode >= 400 {
		var respBody []byte
		if resp.Body != nil {
			var err error
			respBody, err = io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			if err != nil {
				return nil, schema.NewConnectorError(http.StatusInternalServerError, resp.Status, map[string]any{
					"error": err,
				})
			}
		}

		return nil, schema.NewConnectorError(resp.StatusCode, resp.Status, map[string]any{
			"error": string(respBody),
		})
	}

	if resp.StatusCode == http.StatusNoContent {
		return true, nil
	}

	if resp.Body == nil {
		return nil, nil
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	switch contentType {
	case "text/plain", "text/html":
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}
		return string(respBody), nil
	case contentTypeJSON:
		var result any
		err := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if err != nil {
			return nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}
		if selection == nil {
			return result, nil
		}

		return utils.EvalNestedColumnFields(selection, result)
	default:
		return nil, schema.NewConnectorError(http.StatusInternalServerError, "failed to evaluate response", map[string]any{
			"cause": fmt.Sprintf("unsupported content type %s", contentType),
		})
	}
}

func parseContentType(input string) string {
	if input == "" {
		return ""
	}
	parts := strings.Split(input, ";")
	return strings.TrimSpace(parts[0])
}
