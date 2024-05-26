package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"slices"
	"strings"
	"time"

	rest "github.com/hasura/ndc-rest-schema/schema"
)

// RetryableRequest wraps the raw request with retryable
type RetryableRequest struct {
	RawRequest  *rest.Request
	ServerID    string
	ContentType string
	Headers     http.Header
	Body        io.ReadSeeker
}

// CreateRequest creates an HTTP request with body copied
func (r *RetryableRequest) CreateRequest(ctx context.Context) (*http.Request, context.CancelFunc, error) {
	if r.Body != nil {
		_, err := r.Body.Seek(0, io.SeekStart)
		if err != nil {
			return nil, nil, err
		}
	}

	ctxR, cancel := context.WithTimeout(ctx, time.Duration(r.RawRequest.Timeout)*time.Second)
	request, err := http.NewRequestWithContext(ctxR, strings.ToUpper(r.RawRequest.Method), r.RawRequest.URL, r.Body)
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

func getHostFromServers(servers []rest.ServerConfig, serverIDs []string) (string, string) {
	var results []string
	var selectedServerIDs []string
	for _, server := range servers {
		if len(serverIDs) > 0 && !slices.Contains(serverIDs, server.ID) {
			continue
		}
		hostPtr := server.URL.Value()
		if hostPtr != nil && *hostPtr != "" {
			results = append(results, *hostPtr)
			selectedServerIDs = append(selectedServerIDs, server.ID)
		}
	}

	switch len(results) {
	case 0:
		return "", ""
	case 1:
		return results[0], selectedServerIDs[0]
	default:
		index := rand.Intn(len(results) - 1)
		return results[index], selectedServerIDs[index]
	}
}

func buildDistributedRequestsWithOptions(request *RetryableRequest, restOptions *RESTOptions) ([]*RetryableRequest, error) {
	if strings.HasPrefix(request.RawRequest.URL, "http") {
		return []*RetryableRequest{request}, nil
	}
	if !restOptions.Distributed || len(restOptions.Settings.Servers) == 1 {
		host, serverID := getHostFromServers(restOptions.Settings.Servers, restOptions.ServerIDs)
		request.RawRequest.URL = fmt.Sprintf("%s%s", host, request.RawRequest.URL)
		request.ServerID = serverID
		return []*RetryableRequest{request}, nil
	}

	var requests []*RetryableRequest
	var buf []byte
	var err error
	if restOptions.Parallel && request.Body != nil {
		// copy new readers for each requests to avoid race condition
		buf, err = io.ReadAll(request.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %s", err)
		}
	}
	serverIDs := restOptions.ServerIDs
	if len(serverIDs) == 0 {
		for _, server := range restOptions.Settings.Servers {
			serverIDs = append(serverIDs, server.ID)
		}
	}
	for _, serverID := range serverIDs {
		host, serverID := getHostFromServers(restOptions.Settings.Servers, []string{serverID})
		if host == "" {
			continue
		}
		req := &RetryableRequest{
			ServerID:    serverID,
			RawRequest:  request.RawRequest.Clone(),
			ContentType: request.ContentType,
			Headers:     request.Headers,
			Body:        request.Body,
		}
		req.RawRequest.URL = fmt.Sprintf("%s%s", host, req.RawRequest.URL)
		if len(buf) > 0 {
			req.Body = bytes.NewReader(buf)
		}
		requests = append(requests, req)
	}
	return requests, nil
}
