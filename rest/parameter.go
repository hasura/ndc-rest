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

func (c *RESTConnector) evalURLAndHeaderParameters(request *rest.Request, argumentsSchema map[string]schema.ArgumentInfo, arguments map[string]any, headers http.Header) (string, http.Header, error) {
	endpoint, err := url.Parse(request.URL)
	if err != nil {
		return "", nil, err
	}
	for k, h := range request.Headers {
		v := h.Value()
		if v != nil && *v != "" {
			headers.Add(k, *v)
		}
	}

	for _, param := range request.Parameters {
		argName := param.ArgumentName
		if argName == "" {
			argName = param.Name
		}
		_, schemaOk := argumentsSchema[argName]
		if !schemaOk {
			continue
		}
		value, ok := arguments[argName]

		if !ok || value == nil {
			if param.Schema != nil && !param.Schema.Nullable {
				return "", nil, fmt.Errorf("argument %s is required", argName)
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
	// https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md#parameter-object
	switch param.In {
	case rest.InHeader:
		setHeaderParameters(header, param, queryParams)
	case rest.InQuery:
		q := endpoint.Query()
		for _, qp := range queryParams {
			evalQueryParameterURL(&q, param.Name, param.EncodingObject, qp.Keys(), qp.Values())
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

func (c *RESTConnector) encodeParameterValues(typeSchema *rest.TypeSchema, value any, fieldPaths []string) (internal.ParameterItems, error) {

	results := internal.ParameterItems{}
	reflectValue := reflect.ValueOf(value)

	if reflectValue.Kind() == reflect.Invalid {
		return results, nil
	}
	reflectValue, ok := sdkUtils.UnwrapPointerFromReflectValue(reflectValue)
	if !ok {
		if typeSchema.Nullable {
			return results, nil
		} else {
			return nil, fmt.Errorf("parameter %s is required", strings.Join(fieldPaths, ""))
		}
	}

	value = reflectValue.Interface()

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
				results.Add(append([]internal.Key{internal.NewKey(k)}, pair.Keys()...), pair.Values())
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
				keys := output.Keys()
				if len(keys) == 0 {
					results.Add([]internal.Key{internal.NewKey("")}, output.Values())
				} else {
					results.Add(append([]internal.Key{internal.NewIndexKey(i)}, output.Keys()...), output.Values())
				}
			}
		}
		return results, nil
	case string(schema.TypeRepresentationTypeString), string(schema.TypeRepresentationTypeDate), string(schema.TypeRepresentationTypeTimestamp), string(schema.TypeRepresentationTypeTimestampTZ), string(schema.TypeRepresentationTypeUUID), string(schema.TypeRepresentationTypeBytes), string(rest.ScalarBytes), string(rest.ScalarBinary):
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

				return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, []string{valueStr})}, nil
			case *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64, *schema.TypeRepresentationBigDecimal:
				return encodeParameterInt(value, fieldPaths)
			case *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64:
				return encodeParameterFloat(value, fieldPaths)
			}
		}

		return encodeParameterReflectionValues(reflectValue, fieldPaths)
	}
}

func encodeParameterReflectionValues(reflectValue reflect.Value, fieldPaths []string) (internal.ParameterItems, error) {

	results := internal.ParameterItems{}
	reflectValue, ok := sdkUtils.UnwrapPointerFromReflectValue(reflectValue)
	if !ok {
		return results, nil
	}

	kind := reflectValue.Kind()
	switch kind {
	case reflect.Bool:
		return []internal.ParameterItem{
			internal.NewParameterItem([]internal.Key{}, []string{strconv.FormatBool(reflectValue.Bool())}),
		}, nil
	case reflect.String:
		return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, []string{reflectValue.String()})}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return []internal.ParameterItem{
			internal.NewParameterItem([]internal.Key{}, []string{strconv.FormatInt(reflectValue.Int(), 10)}),
		}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []internal.ParameterItem{
			internal.NewParameterItem([]internal.Key{}, []string{strconv.FormatUint(reflectValue.Uint(), 10)}),
		}, nil
	case reflect.Float32, reflect.Float64:
		return []internal.ParameterItem{
			internal.NewParameterItem([]internal.Key{}, []string{fmt.Sprintf("%f", reflectValue.Float())}),
		}, nil
	case reflect.Slice:
		valueLen := reflectValue.Len()
		for i := 0; i < valueLen; i++ {
			propPaths := append(fieldPaths, fmt.Sprintf("[%d]", i))
			elem := reflectValue.Index(i)

			outputs, err := encodeParameterReflectionValues(elem, propPaths)
			if err != nil {
				return nil, err
			}

			for _, output := range outputs {
				keys := output.Keys()
				if len(keys) == 0 {
					results.Add([]internal.Key{internal.NewKey("")}, output.Values())
				} else {
					results.Add(append([]internal.Key{internal.NewIndexKey(i)}, output.Keys()...), output.Values())
				}
			}
		}
		return results, nil
	case reflect.Map:
		keys := reflectValue.MapKeys()
		if len(keys) == 0 {
			return results, nil
		}

		for _, k := range keys {
			key := fmt.Sprint(k.Interface())
			propPaths := append(fieldPaths, fmt.Sprintf(".%s", key))
			fieldVal := reflectValue.MapIndex(k)

			output, err := encodeParameterReflectionValues(fieldVal, propPaths)
			if err != nil {
				return nil, err
			}

			for _, pair := range output {
				results.Add(append([]internal.Key{internal.NewKey(key)}, pair.Keys()...), pair.Values())
			}
		}

		return results, nil
	case reflect.Struct, reflect.Interface:
		b, err := json.Marshal(reflectValue.Interface())
		if err != nil {
			return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
		}
		values := []string{strings.Trim(string(b), `"`)}
		return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, values)}, nil
	default:
		return nil, fmt.Errorf("%s: failed to encode parameter, got %s", strings.Join(fieldPaths, ""), kind)
	}
}

func buildParamQueryKey(name string, encObject rest.EncodingObject, keys internal.Keys, values []string) string {
	resultKeys := []string{}
	if name != "" {
		resultKeys = append(resultKeys, name)
	}
	// non-explode or explode form object does not require param name
	// /users?role=admin&firstName=Alex
	if (encObject.Explode != nil && !*encObject.Explode) ||
		(len(values) == 1 && encObject.Style == rest.EncodingStyleForm && (len(keys) > 1 || (len(keys) == 1 && !keys[0].IsEmpty()))) {
		resultKeys = []string{}
	}

	if len(keys) > 0 {
		if encObject.Style != rest.EncodingStyleDeepObject && keys[len(keys)-1].IsEmpty() {
			keys = keys[:len(keys)-1]
		}
		for _, k := range keys {
			if len(resultKeys) == 0 {
				resultKeys = append(resultKeys, k.String())
			} else {
				resultKeys = append(resultKeys, fmt.Sprintf("[%s]", k))
			}
		}
	}

	return strings.Join(resultKeys, "")
}

func evalQueryParameterURL(q *url.Values, name string, encObject rest.EncodingObject, keys internal.Keys, values []string) {
	if len(values) == 0 {
		return
	}
	paramKey := buildParamQueryKey(name, encObject, keys, values)

	// encode explode queries, e.g /users?id=3&id=4&id=5
	if encObject.Explode == nil || *encObject.Explode {
		for _, value := range values {
			q.Add(paramKey, value)
		}
		return
	}

	switch encObject.Style {
	case rest.EncodingStyleSpaceDelimited:
		q.Add(name, strings.Join(values, " "))
	case rest.EncodingStylePipeDelimited:
		q.Add(name, strings.Join(values, "|"))
	// default style is form
	default:
		paramValues := values
		if paramKey != "" {
			paramValues = append([]string{paramKey}, paramValues...)
		}
		q.Add(name, strings.Join(paramValues, ","))
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

func encodeParameterBool(value any, fieldPaths []string) (internal.ParameterItems, error) {
	result, err := sdkUtils.DecodeBoolean(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
	}

	return []internal.ParameterItem{
		internal.NewParameterItem([]internal.Key{}, []string{strconv.FormatBool(result)}),
	}, nil
}

func encodeParameterString(value any, fieldPaths []string) (internal.ParameterItems, error) {
	result, err := sdkUtils.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
	}
	return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, []string{result})}, nil
}

func encodeParameterInt(value any, fieldPaths []string) (internal.ParameterItems, error) {
	intValue, err := sdkUtils.DecodeInt[int64](value)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
	}
	return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, []string{strconv.FormatInt(intValue, 10)})}, nil
}

func encodeParameterFloat(value any, fieldPaths []string) (internal.ParameterItems, error) {
	floatValue, err := sdkUtils.DecodeFloat[float64](value)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(fieldPaths, ""), err)
	}
	return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, []string{fmt.Sprintf("%f", floatValue)})}, nil
}

func setHeaderParameters(header *http.Header, param *rest.RequestParameter, queryParams internal.ParameterItems) {
	defaultParam := queryParams.FindDefault()
	// the param is an array
	if defaultParam != nil {
		header.Set(param.Name, strings.Join(defaultParam.Values(), ","))
		return
	}

	if param.Explode != nil && *param.Explode {
		var headerValues []string
		for _, pair := range queryParams {
			headerValues = append(headerValues, fmt.Sprintf("%s=%s", pair.Keys().String(), strings.Join(pair.Values(), ",")))
		}
		header.Set(param.Name, strings.Join(headerValues, ","))
		return
	}

	var headerValues []string
	for _, pair := range queryParams {
		pairKey := pair.Keys().String()
		for _, v := range pair.Values() {
			headerValues = append(headerValues, pairKey, v)
		}
	}
	header.Set(param.Name, strings.Join(headerValues, ","))
}
