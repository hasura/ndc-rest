package internal

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// IsSensitiveHeader checks if the header name is sensitive.
func IsSensitiveHeader(name string) bool {
	return sensitiveHeaderRegex.MatchString(strings.ToLower(name))
}

func evalAcceptContentType(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image/*"
	case strings.HasPrefix(contentType, "video/"):
		return "video/*"
	default:
		return contentType
	}
}

func setHeaderAttributes(span trace.Span, prefix string, httpHeaders http.Header) {
	for key, headers := range httpHeaders {
		if len(headers) == 0 {
			continue
		}
		values := headers
		if IsSensitiveHeader(key) {
			values = make([]string, len(headers))
			for i, header := range headers {
				values[i] = utils.MaskString(header)
			}
		}
		span.SetAttributes(attribute.StringSlice(prefix+strings.ToLower(key), values))
	}
}

func evalForwardedHeaders(req *RetryableRequest, headers map[string]string) error {
	for key, value := range headers {
		if req.Headers.Get(key) != "" {
			continue
		}
		req.Headers.Set(key, value)
	}

	return nil
}

func cloneURL(input *url.URL) *url.URL {
	return &url.URL{
		Scheme:      input.Scheme,
		Opaque:      input.Opaque,
		User:        input.User,
		Host:        input.Host,
		Path:        input.Path,
		RawPath:     input.RawPath,
		OmitHost:    input.OmitHost,
		ForceQuery:  input.ForceQuery,
		RawQuery:    input.RawQuery,
		Fragment:    input.Fragment,
		RawFragment: input.RawFragment,
	}
}
