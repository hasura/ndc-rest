package internal

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UnwrapNullableType unwraps the underlying type of the nullable type
func UnwrapNullableType(input schema.Type) (schema.TypeEncoder, bool, error) {
	switch ty := input.Interface().(type) {
	case *schema.NullableType:
		childType, _, err := UnwrapNullableType(ty.UnderlyingType)
		if err != nil {
			return nil, false, err
		}

		return childType, true, nil
	case *schema.NamedType, *schema.ArrayType:
		return ty, false, nil
	default:
		return nil, false, fmt.Errorf("invalid type %v", input)
	}
}

func setHeaderAttributes(span trace.Span, prefix string, httpHeaders http.Header) {
	for key, headers := range httpHeaders {
		if len(headers) == 0 {
			continue
		}
		values := headers
		if sensitiveHeaderRegex.MatchString(strings.ToLower(key)) {
			values = make([]string, len(headers))
			for i, header := range headers {
				values[i] = utils.MaskString(header)
			}
		}
		span.SetAttributes(attribute.StringSlice(prefix+strings.ToLower(key), values))
	}
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
