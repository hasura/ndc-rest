package internal

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

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

// either masks the string value for security
func eitherMaskSecret(input string, shouldMask bool) string {
	if !shouldMask {
		return input
	}

	return MaskString(input)
}

// MaskString masks the string value for security
func MaskString(input string) string {
	inputLength := len(input)
	switch {
	case inputLength < 6:
		return strings.Repeat("*", inputLength)
	case inputLength < 12:
		return input[0:1] + strings.Repeat("*", inputLength-1)
	default:
		return input[0:3] + strings.Repeat("*", 7) + fmt.Sprintf("(%d)", inputLength)
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
				values[i] = MaskString(header)
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

func marshalSimpleScalar(val reflect.Value, kind reflect.Kind) (string, error) {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(val.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(val.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(val.Float(), 'g', -1, val.Type().Bits()), nil
	case reflect.String:
		return val.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(val.Bool()), nil
	case reflect.Interface:
		return fmt.Sprint(val.Interface()), nil
	default:
		return "", fmt.Errorf("invalid value: %v", val.Interface())
	}
}
