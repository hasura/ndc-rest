package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/rest/internal"
	"github.com/hasura/ndc-sdk-go/schema"
	sdkUtils "github.com/hasura/ndc-sdk-go/utils"
)

func (c *RESTConnector) evalURLAndHeaderParameters(request *rest.Request, argumentsSchema map[string]schema.ArgumentInfo, arguments map[string]any) (string, http.Header, error) {
	endpoint, err := url.Parse(request.URL)
	if err != nil {
		return "", nil, err
	}
	headers := http.Header{}
	for k, h := range request.Headers {
		headers.Add(k, h)
	}

	for _, param := range request.Parameters {
		_, schemaOk := argumentsSchema[param.Name]
		if !schemaOk {
			continue
		}
		value, ok := arguments[param.Name]

		if !ok || value == nil {
			if param.Schema != nil && !param.Schema.Nullable {
				return "", nil, fmt.Errorf("parameter %s is required", param.Name)
			}
		} else if err := c.evalURLAndHeaderParameterBySchema(endpoint, &headers, &param, value); err != nil {
			return "", nil, err
		}
	}
	return endpoint.String(), headers, nil
}

// the query parameters serialization follows [OAS 3.1 spec]
//
// [OAS 3.1 spec]: https://swagger.io/docs/specification/serialization/
func (c *RESTConnector) evalURLAndHeaderParameterBySchema(endpoint *url.URL, header *http.Header, param *rest.RequestParameter, value any) error {

	queryParams, err := c.encodeParameterValues(param.Schema, value, []string{param.Name})
	if err != nil {
		return err
	}

	if len(queryParams) == 0 {
		return nil
	}

	// following the OAS spec to serialize parameters
	// https://swagger.io/docs/specification/serialization/
	switch param.In {
	case rest.InHeader:
		defaultParam := queryParams.FindDefault()
		// the param is an array
		if defaultParam != nil {
			header.Set(param.Name, strings.Join(defaultParam.Values(), ","))
			return nil
		}

		if param.Explode != nil && *param.Explode {
			var headerValues []string
			for _, pair := range queryParams {
				headerValues = append(headerValues, fmt.Sprintf("%s=%s", strings.Join(pair.Keys(), ","), strings.Join(pair.Values(), ",")))
			}
			header.Set(param.Name, strings.Join(headerValues, ","))
			return nil
		}

		var headerValues []string
		for _, pair := range queryParams {
			headerValues = append(headerValues, strings.Join(pair.Keys(), ","), strings.Join(pair.Values(), ","))
		}
		header.Set(param.Name, strings.Join(headerValues, ","))
	case rest.InQuery:
		q := endpoint.Query()
		for _, qp := range queryParams {
			evalQueryParameterURL(&q, param, qp.Keys(), qp.Values())
		}
		endpoint.RawQuery = encodeQueryValues(q, param.AllowReserved)
	case rest.InPath:
		defaultParam := queryParams.FindDefault()
		if defaultParam != nil {
			endpoint.Path = strings.ReplaceAll(endpoint.Path, fmt.Sprintf("{%s}", param.Name), strings.Join(defaultParam.Values(), ","))
		}
	}
	return nil
}

func (c *RESTConnector) encodeParameterValues(typeSchema *rest.TypeSchema, value any, fieldPaths []string) (internal.StringSlicePairs, error) {

	results := internal.StringSlicePairs{}
	reflectValue := reflect.ValueOf(value)

	if reflectValue.Kind() == reflect.Invalid {
		return results, nil
	}
	if reflectValue.Kind() == reflect.Ptr {
		if reflectValue.IsNil() {
			if typeSchema.Nullable {
				return results, nil
			} else {
				return nil, fmt.Errorf("parameter %s is required", strings.Join(fieldPaths, ""))
			}
		}
		value = reflectValue.Elem().Interface()
	}
	switch strings.ToLower(typeSchema.Type) {
	case "object":
		if len(typeSchema.Properties) == 0 {
			return results, nil
		}
		mapValue, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: expected object, got %v", strings.Join(fieldPaths, ""), value)
		}

		for k, prop := range typeSchema.Properties {
			propPaths := append(fieldPaths, fmt.Sprintf(".%s", k))
			fieldVal, ok := mapValue[k]
			if !ok {
				if !prop.Nullable {
					return nil, fmt.Errorf("parameter %s is required", strings.Join(propPaths, ""))
				}
				continue
			}

			output, err := c.encodeParameterValues(&prop, fieldVal, propPaths)
			if err != nil {
				return nil, err
			}
			for _, pair := range output {
				results = append(results, internal.NewStringSlicePair(append([]string{k}, pair.Keys()...), pair.Values()))
			}
		}

		return results, nil
	case "array":
		arrayValue, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("%s: expected array, got %v", strings.Join(fieldPaths, ""), value)
		}

		for i, itemValue := range arrayValue {
			propPaths := append(fieldPaths, fmt.Sprintf("[%d]", i))

			outputs, err := c.encodeParameterValues(typeSchema.Items, itemValue, propPaths)
			if err != nil {
				return nil, err
			}

			for _, output := range outputs {
				results.Add(append([]string{""}, output.Keys()...), output.Values())
			}
		}
		return results, nil
	case string(schema.TypeRepresentationTypeString), string(schema.TypeRepresentationTypeDate), string(schema.TypeRepresentationTypeTimestamp), string(schema.TypeRepresentationTypeTimestampTZ), string(schema.TypeRepresentationTypeBytes), string(schema.TypeRepresentationTypeUUID):
		return encodeParameterString(value, fieldPaths)
	case string(schema.TypeRepresentationTypeInt32), string(schema.TypeRepresentationTypeInt64), strings.ToLower(string(rest.ScalarUnixTime)), "integer", "long":
		return encodeParameterInt(value, fieldPaths)
	case string(schema.TypeRepresentationTypeFloat32), string(schema.TypeRepresentationTypeFloat64), "float":
		return encodeParameterFloat(value, fieldPaths)
	case string(schema.TypeRepresentationTypeBoolean), string(rest.ScalarBoolean):
		return encodeParameterBool(value, fieldPaths)
	default:
		iScalar, ok := c.schema.ScalarTypes[typeSchema.Type]
		if ok {
			switch ty := iScalar.Representation.Interface().(type) {
			case *schema.TypeRepresentationBoolean:
				return encodeParameterBool(value, fieldPaths)
			case *schema.TypeRepresentationString, *schema.TypeRepresentationDate, *schema.TypeRepresentationTimestamp, *schema.TypeRepresentationTimestampTZ, *schema.TypeRepresentationBytes, *schema.TypeRepresentationUUID:
				return encodeParameterString(value, fieldPaths)
			case *schema.TypeRepresentationEnum:
				valueStr, err := sdkUtils.DecodeString(value)
				if err != nil {
					return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
				}

				if !slices.Contains(ty.OneOf, valueStr) {
					return nil, fmt.Errorf("%s: invalid enum value '%s'", strings.Join(fieldPaths, ""), valueStr)
				}

				return []*internal.StringSlicePair{internal.NewStringSlicePair([]string{}, []string{valueStr})}, nil
			case *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64, *schema.TypeRepresentationBigDecimal:
				return encodeParameterInt(value, fieldPaths)
			case *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64:
				return encodeParameterFloat(value, fieldPaths)
			default:
				return []*internal.StringSlicePair{
					internal.NewStringSlicePair([]string{}, []string{fmt.Sprint(value)}),
				}, nil
			}
		}

		// TODO: encode nested fields without type schema
		b, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
		}
		values := []string{strings.Trim(string(b), `"`)}
		return []*internal.StringSlicePair{internal.NewStringSlicePair([]string{}, values)}, nil
	}
}

func buildParamQueryKey(param *rest.RequestParameter, keys []string, values []string) string {
	resultKeys := []string{param.Name}
	// non-explode or explode form object does not require param name
	// /users?role=admin&firstName=Alex
	if (param.Explode != nil && !*param.Explode) ||
		(len(values) == 1 && param.Style == rest.EncodingStyleForm && (len(keys) > 1 || (len(keys) == 1 && keys[0] != ""))) {
		resultKeys = []string{}
	}

	if len(keys) > 0 {
		if param.Style != rest.EncodingStyleDeepObject && keys[len(keys)-1] == "" {
			keys = keys[:len(keys)-1]
		}
		for _, k := range keys {
			if len(resultKeys) == 0 {
				resultKeys = append(resultKeys, k)
			} else {
				resultKeys = append(resultKeys, fmt.Sprintf("[%s]", k))
			}
		}
	}

	return strings.Join(resultKeys, "")
}

func evalQueryParameterURL(q *url.Values, param *rest.RequestParameter, keys []string, values []string) {
	if len(values) == 0 {
		return
	}
	paramKey := buildParamQueryKey(param, keys, values)

	// encode explode queries, e.g /users?id=3&id=4&id=5
	if param.Explode == nil || *param.Explode {
		for _, value := range values {
			q.Add(paramKey, value)
		}
		return
	}

	switch param.Style {
	case rest.EncodingStyleSpaceDelimited:
		q.Add(param.Name, strings.Join(values, " "))
	case rest.EncodingStylePipeDelimited:
		q.Add(param.Name, strings.Join(values, "|"))
	// default style is form
	default:
		paramValues := values
		if paramKey != "" {
			paramValues = append([]string{paramKey}, paramValues...)
		}
		q.Add(param.Name, strings.Join(paramValues, ","))
	}
}

func encodeQueryValues(qValues url.Values, allowReserved bool) string {
	if !allowReserved {
		return qValues.Encode()
	}

	var builder strings.Builder
	index := 0
	for key, values := range qValues {
		for i, value := range values {
			if index > 0 || i > 0 {
				builder.WriteRune('&')
			}
			builder.WriteString(key)
			builder.WriteRune('=')
			builder.WriteString(value)
		}
		index++
	}
	return builder.String()
}

func encodeParameterBool(value any, fieldPaths []string) (internal.StringSlicePairs, error) {
	result, err := sdkUtils.DecodeBoolean(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
	}

	return []*internal.StringSlicePair{
		internal.NewStringSlicePair([]string{}, []string{strconv.FormatBool(result)}),
	}, nil
}

func encodeParameterString(value any, fieldPaths []string) (internal.StringSlicePairs, error) {
	result, err := sdkUtils.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
	}
	return []*internal.StringSlicePair{internal.NewStringSlicePair([]string{}, []string{result})}, nil
}

func encodeParameterInt(value any, fieldPaths []string) (internal.StringSlicePairs, error) {
	intValue, err := sdkUtils.DecodeInt[int64](value)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
	}
	return []*internal.StringSlicePair{internal.NewStringSlicePair([]string{}, []string{strconv.FormatInt(intValue, 10)})}, nil
}

func encodeParameterFloat(value any, fieldPaths []string) (internal.StringSlicePairs, error) {
	floatValue, err := sdkUtils.DecodeFloat[float64](value)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
	}
	return []*internal.StringSlicePair{internal.NewStringSlicePair([]string{}, []string{fmt.Sprintf("%f", floatValue)})}, nil
}
