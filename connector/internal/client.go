package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
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
func (client *HTTPClient) Send(ctx context.Context, request *RetryableRequest, selection schema.NestedField, resultType schema.Type, httpOptions *HTTPOptions) (any, http.Header, error) {
	requests, err := BuildDistributedRequestsWithOptions(request, httpOptions)
	if err != nil {
		return nil, nil, err
	}

	if !httpOptions.Distributed {
		result, headers, err := client.sendSingle(ctx, &requests[0], selection, resultType, requests[0].ServerID, "single")
		if err != nil {
			return nil, nil, err
		}

		return result, headers, nil
	}

	if !httpOptions.Parallel || httpOptions.Concurrency <= 1 {
		results, headers := client.sendSequence(ctx, requests, selection, resultType)

		return results, headers, nil
	}

	results, headers := client.sendParallel(ctx, requests, selection, resultType, httpOptions)

	return results, headers, nil
}

// execute a request to a list of remote servers in sequence
func (client *HTTPClient) sendSequence(ctx context.Context, requests []RetryableRequest, selection schema.NestedField, resultType schema.Type) (*DistributedResponse[any], http.Header) {
	results := NewDistributedResponse[any]()
	var firstHeaders http.Header

	for _, req := range requests {
		result, headers, err := client.sendSingle(ctx, &req, selection, resultType, req.ServerID, "sequence")
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
func (client *HTTPClient) sendParallel(ctx context.Context, requests []RetryableRequest, selection schema.NestedField, resultType schema.Type, httpOptions *HTTPOptions) (*DistributedResponse[any], http.Header) {
	results := NewDistributedResponse[any]()
	var firstHeaders http.Header
	var lock sync.Mutex
	eg, ctx := errgroup.WithContext(ctx)
	if httpOptions.Concurrency > 0 {
		eg.SetLimit(int(httpOptions.Concurrency))
	}

	sendFunc := func(req RetryableRequest) {
		eg.Go(func() error {
			result, headers, err := client.sendSingle(ctx, &req, selection, resultType, req.ServerID, "parallel")
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
func (client *HTTPClient) sendSingle(ctx context.Context, request *RetryableRequest, selection schema.NestedField, resultType schema.Type, serverID string, mode string) (any, http.Header, *schema.ConnectorError) {
	ctx, span := client.tracer.Start(ctx, "Send Request to Server "+serverID)
	defer span.End()

	span.SetAttributes(attribute.String("execution.mode", mode))

	requestURL := request.URL.String()
	rawPort := request.URL.Port()
	port := 80
	if rawPort != "" {
		if p, err := strconv.ParseInt(rawPort, 10, 32); err == nil {
			port = int(p)
		}
	} else if strings.HasPrefix(request.URL.Scheme, "https") {
		port = 443
	}

	client.propagator.Inject(ctx, propagation.HeaderCarrier(request.Headers))

	logger := connector.GetLogger(ctx)
	if logger.Enabled(ctx, slog.LevelDebug) {
		logAttrs := []any{
			slog.String("request_url", requestURL),
			slog.String("request_method", request.RawRequest.Method),
			slog.Any("request_headers", request.Headers),
		}

		if request.Body != nil {
			bs, _ := io.ReadAll(request.Body)
			logAttrs = append(logAttrs, slog.String("request_body", string(bs)))
		}
		logger.Debug("sending request to remote server...", logAttrs...)
	}

	var resp *http.Response
	var errorBytes []byte
	var err error
	var cancel context.CancelFunc

	times := int(request.Runtime.Retry.Times)
	delayMs := int(math.Max(float64(request.Runtime.Retry.Delay), 100))
	for i := 0; i <= times; i++ {
		resp, errorBytes, cancel, err = client.doRequest(ctx, request, port, i) //nolint:all
		if err != nil {
			span.SetStatus(codes.Error, "failed to execute the request")
			span.RecordError(err)

			return nil, nil, schema.NewConnectorError(http.StatusInternalServerError, err.Error(), nil)
		}

		if (resp.StatusCode >= 200 && resp.StatusCode < 299) ||
			!slices.Contains(request.Runtime.Retry.HTTPStatus, resp.StatusCode) || i >= times {
			break
		}

		if logger.Enabled(ctx, slog.LevelDebug) {
			logger.Debug(
				fmt.Sprintf("received error from remote server, retry %d of %d...", i+1, times),
				slog.Int("http_status", resp.StatusCode),
				slog.Any("response_headers", resp.Header),
				slog.String("response_body", string(errorBytes)),
			)
		}

		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}

	defer cancel()

	contentType := parseContentType(resp.Header.Get(contentTypeHeader))
	if resp.StatusCode >= 400 {
		details := make(map[string]any)
		if contentType == rest.ContentTypeJSON && json.Valid(errorBytes) {
			details["error"] = json.RawMessage(errorBytes)
		} else {
			details["error"] = string(errorBytes)
		}

		span.SetStatus(codes.Error, "received error from remote server")

		return nil, nil, schema.NewConnectorError(resp.StatusCode, resp.Status, details)
	}

	result, headers, evalErr := evalHTTPResponse(ctx, span, resp, contentType, selection, resultType, logger)
	if evalErr != nil {
		span.SetStatus(codes.Error, "failed to decode the http response")
		span.RecordError(evalErr)

		return nil, nil, evalErr
	}

	return result, headers, nil
}

func (client *HTTPClient) doRequest(ctx context.Context, request *RetryableRequest, port int, retryCount int) (*http.Response, []byte, context.CancelFunc, error) {
	method := strings.ToUpper(request.RawRequest.Method)
	ctx, span := client.tracer.Start(ctx, fmt.Sprintf("%s %s", method, request.RawRequest.URL), trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	urlAttr := cloneURL(&request.URL)
	password, hasPassword := urlAttr.User.Password()
	if urlAttr.User.String() != "" || hasPassword {
		maskedUser := MaskString(urlAttr.User.Username())
		if hasPassword {
			urlAttr.User = url.UserPassword(maskedUser, MaskString(password))
		} else {
			urlAttr.User = url.User(maskedUser)
		}
	}

	span.SetAttributes(
		attribute.String("db.system", "http"),
		attribute.String("http.request.method", method),
		attribute.String("url.full", urlAttr.String()),
		attribute.String("server.address", request.URL.Hostname()),
		attribute.Int("server.port", port),
		attribute.String("network.protocol.name", "http"),
	)

	if request.ContentLength > 0 {
		span.SetAttributes(attribute.Int64("http.request.body.size", request.ContentLength))
	}
	if retryCount > 0 {
		span.SetAttributes(attribute.Int("http.request.resend_count", retryCount))
	}
	setHeaderAttributes(span, "http.request.header.", request.Headers)

	client.propagator.Inject(ctx, propagation.HeaderCarrier(request.Headers))

	req, cancel, err := request.CreateRequest(ctx)
	if err != nil {
		span.SetStatus(codes.Error, "error happened when creating request")
		span.RecordError(err)

		return nil, nil, nil, err
	}

	resp, err := client.client.Do(req)
	if err != nil {
		span.SetStatus(codes.Error, "error happened when executing the request")
		span.RecordError(err)
		cancel()

		return nil, nil, nil, err
	}

	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	setHeaderAttributes(span, "http.response.header.", resp.Header)

	if resp.StatusCode < 300 {
		return resp, nil, cancel, nil
	}

	defer resp.Body.Close()
	span.SetStatus(codes.Error, "Non-2xx status")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
	} else {
		span.RecordError(errors.New(string(body)))
	}

	return resp, body, cancel, nil
}

func evalHTTPResponse(ctx context.Context, span trace.Span, resp *http.Response, contentType string, selection schema.NestedField, resultType schema.Type, logger *slog.Logger) (any, http.Header, *schema.ConnectorError) {
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

	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode == http.StatusNoContent {
		return true, resp.Header, nil
	}

	if resp.Body == nil {
		return nil, resp.Header, nil
	}

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
