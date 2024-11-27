package internal

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
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

// either masks the string value for security
func eitherMaskSecret(input string, shouldMask bool) string {
	if !shouldMask {
		return input
	}

	return utils.MaskString(input)
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

func stringifySimpleScalar(val reflect.Value, kind reflect.Kind) (string, error) {
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

func findXMLLeafObjectField(objectType rest.ObjectType) (*rest.ObjectField, string, bool) {
	var f *rest.ObjectField
	var fieldName string
	for key, field := range objectType.Fields {
		if field.HTTP == nil || field.HTTP.XML == nil {
			return nil, "", false
		}
		if field.HTTP.XML.Text {
			f = &field
			fieldName = key
		} else if !field.HTTP.XML.Attribute {
			return nil, "", false
		}
	}

	return f, fieldName, true
}

func getTypeSchemaXMLName(typeSchema *rest.TypeSchema, defaultName string) string {
	if typeSchema != nil {
		return getXMLName(typeSchema.XML, defaultName)
	}

	return defaultName
}

func getXMLName(xmlSchema *rest.XMLSchema, defaultName string) string {
	if xmlSchema != nil {
		if xmlSchema.Name != "" {
			return xmlSchema.GetFullName()
		}

		if xmlSchema.Prefix != "" {
			return xmlSchema.Prefix + ":" + defaultName
		}
	}

	return defaultName
}

func getArrayOrNamedType(schemaType schema.Type) (*schema.ArrayType, *schema.NamedType, error) {
	rawType, err := schemaType.InterfaceT()
	if err != nil {
		return nil, nil, err
	}

	switch t := rawType.(type) {
	case *schema.NullableType:
		return getArrayOrNamedType(t.UnderlyingType)
	case *schema.ArrayType:
		return t, nil, nil
	case *schema.NamedType:
		return nil, t, nil
	default:
		return nil, nil, nil
	}
}
