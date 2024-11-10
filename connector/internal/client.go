package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

// Doer abstracts a HTTP client with Do method
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPClient represents a http client wrapper with advanced methods
type HTTPClient struct {
	client     Doer
	tracer     *connector.Tracer
	propagator propagation.TextMapPropagator
}

// NewHTTPClient creates a http client wrapper
func NewHTTPClient(client Doer) *HTTPClient {
	return &HTTPClient{
		client:     client,
		propagator: otel.GetTextMapPropagator(),
	}
}

// SetTracer sets the tracer instance
func (client *HTTPClient) SetTracer(tracer *connector.Tracer) {
	client.tracer = tracer
}

// Send creates and executes the request and evaluate response selection
func (client *HTTPClient) Send(ctx context.Context, request *RetryableRequest, selection schema.NestedField, resultType schema.Type, restOptions *RESTOptions) (any, http.Header, error) {
	requests, err := BuildDistributedRequestsWithOptions(request, restOptions)
	if err != nil {
		return nil, nil, err
	}

	if !restOptions.Distributed {
		result, headers, err := client.sendSingle(ctx, &requests[0], selection, resultType)
		if err != nil {
			return nil, nil, err
		}

		return result, headers, nil
	}

	if !restOptions.Parallel || restOptions.Concurrency <= 1 {
		results, headers := client.sendSequence(ctx, requests, selection, resultType)

		return results, headers, nil
	}

	results, headers := client.sendParallel(ctx, requests, selection, resultType, restOptions)

	return results, headers, nil
}

// execute a request to a list of remote servers in sequence
func (client *HTTPClient) sendSequence(ctx context.Context, requests []RetryableRequest, selection schema.NestedField, resultType schema.Type) (*DistributedResponse[any], http.Header) {
	results := NewDistributedResponse[any]()
	var firstHeaders http.Header

	for _, req := range requests {
		result, headers, err := client.sendSingle(ctx, &req, selection, resultType)
		if err != nil {
			results.Errors = append(results.Errors, DistributedError{
				Server:         req.ServerID,
				ConnectorError: *err,
			})
		} else {
			results.Results = append(results.Results, DistributedResult[any]{
				Server: req.ServerID,
				Data:   result,
			})

			if firstHeaders == nil {
				firstHeaders = headers
			}
		}
	}

	return results, firstHeaders
}

// execute a request to a list of remote servers in parallel
func (client *HTTPClient) sendParallel(ctx context.Context, requests []RetryableRequest, selection schema.NestedField, resultType schema.Type, restOptions *RESTOptions) (*DistributedResponse[any], http.Header) {
	results := NewDistributedResponse[any]()
	var firstHeaders http.Header
	var lock sync.Mutex
	eg, ctx := errgroup.WithContext(ctx)
	if restOptions.Concurrency > 0 {
		eg.SetLimit(int(restOptions.Concurrency))
	}

	sendFunc := func(req RetryableRequest) {
		eg.Go(func() error {
			result, headers, err := client.sendSingle(ctx, &req, selection, resultType)
			lock.Lock()
			defer lock.Unlock()
			if err != nil {
				results.Errors = append(results.Errors, DistributedError{
					Server:         req.ServerID,
					ConnectorError: *err,
				})
			} else {
				results.Results = append(results.Results, DistributedResult[any]{
					Server: req.ServerID,
					Data:   result,
				})
				if firstHeaders == nil {
					firstHeaders = headers
				}
			}

			return nil
		})
	}

	for _, req := range requests {
		sendFunc(req)
	}

	_ = eg.Wait()

	return results, firstHeaders
}

// execute a request to the remote server with retries
func (client *HTTPClient) sendSingle(ctx context.Context, request *RetryableRequest, selection schema.NestedField, resultType schema.Type) (any, http.Header, *schema.ConnectorError) {
	ctx, span := client.tracer.Start(ctx, "request_remote_server")
	defer span.End()
	span.SetAttributes(
		attribute.String("request_url", request.URL),
		attribute.String("method", request.RawRequest.Method),
	)
	client.propagator.Inject(ctx, propagation.HeaderCarrier(request.Headers))

	var resp *http.Response
	var err error
	logger := connector.GetLogger(ctx)

	if logger.Enabled(ctx, slog.LevelDebug) {
		logAttrs := []any{
			slog.String("request_url", request.URL),
			slog.String("request_method", request.RawRequest.Method),
			slog.Any("request_headers", request.Headers),
		}
		if request.Body != nil {
			bs, _ := io.ReadAll(request.Body)
			logAttrs = append(logAttrs, slog.String("request_body", string(bs)))
		}
		logger.Debug("sending request to remote server...", logAttrs...)
	}

	times := int(math.Max(float64(request.Runtime.Retry.Times), 1))
	delayMs := int(math.Max(float64(request.Runtime.Retry.Delay), 100))
	for i := range times {
		req, cancel, reqError := request.CreateRequest(ctx)
		if reqError != nil {
			cancel()
			span.SetStatus(codes.Error, "error happened when creating request")
			span.RecordError(err)
			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}
		resp, err = client.client.Do(req)
		if (err == nil && resp.StatusCode >= 200 && resp.StatusCode < 299) || i >= times {
			break
		}

		logAttrs := []any{}
		if err != nil {
			span.AddEvent(fmt.Sprintf("request error, retry %d of %d", i+1, times), trace.WithAttributes(attribute.String("error", err.Error())))
			logAttrs = append(logAttrs, slog.Any("error", err.Error()))
		} else {
			var respBody []byte
			if resp.Body != nil {
				respBody, _ = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
			}

			logAttrs = append(logAttrs,
				slog.Int("http_status", resp.StatusCode),
				slog.Any("response_headers", resp.Header),
				slog.String("response_body", string(respBody)),
			)
			span.AddEvent(
				fmt.Sprintf("received error from remote server, retry %d of %d", i+1, times),
				trace.WithAttributes(
					attribute.Int("http_status", resp.StatusCode),
					attribute.String("response_body", string(respBody)),
				),
			)
		}

		if logger.Enabled(ctx, slog.LevelDebug) {
			logger.Debug(
				fmt.Sprintf("received error from remote server, retry %d of %d...", i+1, times),
				logAttrs...,
			)
		}

		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}

	if err != nil {
		span.SetStatus(codes.Error, "error happened when creating request")
		span.RecordError(err)
		return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
	}

	span.SetAttributes(attribute.Int("http_status", resp.StatusCode))

	return evalHTTPResponse(ctx, span, resp, selection, resultType)
}

func evalHTTPResponse(ctx context.Context, span trace.Span, resp *http.Response, selection schema.NestedField, resultType schema.Type) (any, http.Header, *schema.ConnectorError) {
	logger := connector.GetLogger(ctx)
	contentType := parseContentType(resp.Header.Get(contentTypeHeader))
	if resp.StatusCode >= 400 {
		var respBody []byte
		if resp.Body != nil {
			var err error
			respBody, err = io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			if err != nil {
				span.SetStatus(codes.Error, "error happened when reading response body")
				span.RecordError(err)
				return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, resp.Status, map[string]any{
					"error": err,
				})
			}
		}
		details := make(map[string]any)
		if contentType == rest.ContentTypeJSON && json.Valid(respBody) {
			details["error"] = json.RawMessage(respBody)
		} else {
			details["error"] = string(respBody)
		}

		span.SetAttributes(attribute.String("response_error", string(respBody)))
		span.SetStatus(codes.Error, "received error from remote server")
		return nil, nil, schema.NewConnectorError(resp.StatusCode, resp.Status, details)
	}

	if logger.Enabled(ctx, slog.LevelDebug) {
		logAttrs := []any{
			slog.Int("http_status", resp.StatusCode),
			slog.Any("response_headers", resp.Header),
		}
		if resp.Body != nil && resp.StatusCode != http.StatusNoContent {
			respBody, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				span.SetStatus(codes.Error, "error happened when reading response body")
				span.RecordError(readErr)
				return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, "error happened when reading response body", map[string]any{
					"error": readErr.Error(),
				})
			}
			resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
			logAttrs = append(logAttrs, slog.String("response_body", string(respBody)))
		}
		logger.Debug("received response from remote server", logAttrs...)
	}

	if resp.StatusCode == http.StatusNoContent {
		return true, resp.Header, nil
	}

	if resp.Body == nil {
		return nil, resp.Header, nil
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	switch contentType {
	case "":
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}
		if len(respBody) == 0 {
			return nil, resp.Header, nil
		}
		return string(respBody), resp.Header, nil
	case "text/plain", "text/html":
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}
		return string(respBody), resp.Header, nil
	case rest.ContentTypeJSON:
		if len(resultType) > 0 {
			namedType, err := resultType.AsNamed()
			if err == nil && namedType.Name == string(rest.ScalarString) {
				respBytes, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, "failed to read response", map[string]any{
						"reason": err.Error(),
					})
				}

				var strResult string
				if err := json.Unmarshal(respBytes, &strResult); err != nil {
					// fallback to raw string response if the result type is String
					return string(respBytes), resp.Header, nil
				}
				return strResult, resp.Header, nil
			}
		}

		var result any
		err := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if err != nil {
			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}
		if selection == nil || selection.IsNil() {
			return result, resp.Header, nil
		}

		result, err = utils.EvalNestedColumnFields(selection, result)
		if err != nil {
			return nil, nil, schema.InternalServerError(err.Error(), nil)
		}
		return result, resp.Header, nil
	case rest.ContentTypeNdJSON:
		var results []any
		decoder := json.NewDecoder(resp.Body)
		for decoder.More() {
			var r any
			err := decoder.Decode(&r)
			if err != nil {
				return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
			}
			results = append(results, r)
		}
		if selection == nil || selection.IsNil() {
			return results, resp.Header, nil
		}

		result, err := utils.EvalNestedColumnFields(selection, any(results))
		if err != nil {
			return nil, nil, schema.InternalServerError(err.Error(), nil)
		}
		return result, resp.Header, nil
	default:
		return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, "failed to evaluate response", map[string]any{
			"cause": "unsupported content type " + contentType,
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
