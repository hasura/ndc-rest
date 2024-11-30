package internal

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
)

// RetryableRequest wraps the raw request with retryable
type RetryableRequest struct {
	RawRequest    *rest.Request
	URL           url.URL
	Namespace     string
	ServerID      string
	ContentType   string
	ContentLength int64
	Headers       http.Header
	Body          io.ReadSeeker
	Runtime       rest.RuntimeSettings
}

// CreateRequest creates an HTTP request with body copied
func (r *RetryableRequest) CreateRequest(ctx context.Context) (*http.Request, context.CancelFunc, error) {
	if r.Body != nil {
		_, err := r.Body.Seek(0, io.SeekStart)
		if err != nil {
			return nil, nil, err
		}
	}

	timeout := r.Runtime.Timeout
	if timeout == 0 {
		timeout = defaultTimeoutSeconds
	}

	ctxR, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	request, err := http.NewRequestWithContext(ctxR, strings.ToUpper(r.RawRequest.Method), r.URL.String(), r.Body)
	if err != nil {
		cancel()

		return nil, nil, err
	}
	for key, header := range r.Headers {
		request.Header[key] = header
	}
	request.Header.Set(rest.ContentTypeHeader, r.ContentType)

	return request, cancel, nil
}
