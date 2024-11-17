package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	sdkUtils "github.com/hasura/ndc-sdk-go/utils"
)

var urlAndHeaderLocations = []rest.ParameterLocation{rest.InPath, rest.InQuery, rest.InHeader}

// evaluate URL and header parameters
func (c *RequestBuilder) evalURLAndHeaderParameters() (*url.URL, http.Header, error) {
	endpoint, err := url.Parse(c.Operation.Request.URL)
	if err != nil {
		return nil, nil, err
	}
	headers := http.Header{}
	for k, h := range c.Operation.Request.Headers {
		v, err := h.Get()
		if err != nil {
			return nil, nil, fmt.Errorf("invalid header value, key: %s, %w", k, err)
		}
		if v != "" {
			headers.Add(k, v)
		}
	}

	for argumentKey, argumentInfo := range c.Operation.Arguments {
		if argumentInfo.HTTP == nil || !slices.Contains(urlAndHeaderLocations, argumentInfo.HTTP.In) {
			continue
		}
		if err := c.evalURLAndHeaderParameterBySchema(endpoint, &headers, argumentKey, &argumentInfo, c.Arguments[argumentKey]); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", argumentKey, err)
		}
	}

	return endpoint, headers, nil
}

// the query parameters serialization follows [OAS 3.1 spec]
//
// [OAS 3.1 spec]: https://swagger.io/docs/specification/serialization/
func (c *RequestBuilder) evalURLAndHeaderParameterBySchema(endpoint *url.URL, header *http.Header, argumentKey string, argumentInfo *rest.ArgumentInfo, value any) error {
	if argumentInfo.HTTP.Name != "" {
		argumentKey = argumentInfo.HTTP.Name
	}
	queryParams, err := c.encodeParameterValues(&rest.ObjectField{
		ObjectField: schema.ObjectField{
			Type: argumentInfo.Type,
		},
		HTTP: argumentInfo.HTTP.Schema,
	}, reflect.ValueOf(value), []string{argumentKey})
	if err != nil {
		return err
	}

	if len(queryParams) == 0 {
		return nil
	}

	// following the OAS spec to serialize parameters
	// https://swagger.io/docs/specification/serialization/
	// https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md#parameter-object
	switch argumentInfo.HTTP.In {
	case rest.InHeader:
		setHeaderParameters(header, argumentInfo.HTTP, queryParams)
	case rest.InQuery:
		q := endpoint.Query()
		for _, qp := range queryParams {
			evalQueryParameterURL(&q, argumentKey, argumentInfo.HTTP.EncodingObject, qp.Keys(), qp.Values())
		}
		endpoint.RawQuery = encodeQueryValues(q, argumentInfo.HTTP.AllowReserved)
	case rest.InPath:
		defaultParam := queryParams.FindDefault()
		if defaultParam != nil {
			endpoint.Path = strings.ReplaceAll(endpoint.Path, "{"+argumentKey+"}", strings.Join(defaultParam.Values(), ","))
		}
	}

	return nil
}

func (c *RequestBuilder) encodeParameterValues(objectField *rest.ObjectField, reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	results := ParameterItems{}

	typeSchema := objectField.HTTP
	reflectValue, nonNull := sdkUtils.UnwrapPointerFromReflectValue(reflectValue)

	switch ty := objectField.Type.Interface().(type) {
	case *schema.NullableType:
		if !nonNull {
			return results, nil
		}

		return c.encodeParameterValues(&rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: ty.UnderlyingType,
			},
			HTTP: typeSchema,
		}, reflectValue, fieldPaths)
	case *schema.ArrayType:
		if !nonNull {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), errArgumentRequired)
		}

		elements, ok := reflectValue.Interface().([]any)
		if !ok {
			return nil, fmt.Errorf("%s: expected array, got <%s> %v", strings.Join(fieldPaths, ""), reflectValue.Kind(), reflectValue.Interface())
		}

		for i, elem := range elements {
			outputs, err := c.encodeParameterValues(&rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: ty.ElementType,
				},
				HTTP: typeSchema.Items,
			}, reflect.ValueOf(elem), append(fieldPaths, "["+strconv.Itoa(i)+"]"))
			if err != nil {
				return nil, err
			}

			for _, output := range outputs {
				results.Add(append([]Key{NewIndexKey(i)}, output.Keys()...), output.Values())
			}
		}

		return results, nil
	case *schema.NamedType:
		if !nonNull {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), errArgumentRequired)
		}
		iScalar, ok := c.Schema.ScalarTypes[ty.Name]
		if ok {
			return encodeScalarParameterReflectionValues(reflectValue, &iScalar, fieldPaths)
		}
		kind := reflectValue.Kind()
		objectInfo, ok := c.Schema.ObjectTypes[ty.Name]
		if !ok {
			return nil, fmt.Errorf("%s: invalid type %s", strings.Join(fieldPaths, ""), ty.Name)
		}

		switch kind {
		case reflect.Map, reflect.Interface:
			anyValue := reflectValue.Interface()
			object, ok := anyValue.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s: failed to evaluate object, got <%s> %v", strings.Join(fieldPaths, ""), kind, anyValue)
			}
			for key, fieldInfo := range objectInfo.Fields {
				fieldVal := object[key]
				output, err := c.encodeParameterValues(&fieldInfo, reflect.ValueOf(fieldVal), append(fieldPaths, "."+key))
				if err != nil {
					return nil, err
				}

				for _, pair := range output {
					results.Add(append([]Key{NewKey(key)}, pair.Keys()...), pair.Values())
				}
			}
		case reflect.Struct:
			reflectType := reflectValue.Type()
			for fieldIndex := range reflectValue.NumField() {
				fieldVal := reflectValue.Field(fieldIndex)
				fieldType := reflectType.Field(fieldIndex)
				fieldInfo, ok := objectInfo.Fields[fieldType.Name]
				if !ok {
					continue
				}

				output, err := c.encodeParameterValues(&fieldInfo, fieldVal, append(fieldPaths, "."+fieldType.Name))
				if err != nil {
					return nil, err
				}

				for _, pair := range output {
					results.Add(append([]Key{NewKey(fieldType.Name)}, pair.Keys()...), pair.Values())
				}
			}
		default:
			return nil, fmt.Errorf("%s: failed to evaluate object, got %s", strings.Join(fieldPaths, ""), kind)
		}

		return results, nil
	}

	return nil, fmt.Errorf("%s: invalid type %v", strings.Join(fieldPaths, ""), objectField.Type)
}

func encodeScalarParameterReflectionValues(reflectValue reflect.Value, scalar *schema.ScalarType, fieldPaths []string) (ParameterItems, error) {
	switch sl := scalar.Representation.Interface().(type) {
	case *schema.TypeRepresentationBoolean:
		value, err := utils.DecodeBooleanReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{strconv.FormatBool(value)}),
		}, nil
	case *schema.TypeRepresentationString, *schema.TypeRepresentationBytes:
		value, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{NewParameterItem([]Key{}, []string{value})}, nil
	case *schema.TypeRepresentationInteger, *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64, *schema.TypeRepresentationBigInteger: //nolint:all
		value, err := utils.DecodeIntReflection[int64](reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{strconv.FormatInt(value, 10)}),
		}, nil
	case *schema.TypeRepresentationNumber, *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64, *schema.TypeRepresentationBigDecimal: //nolint:all
		value, err := utils.DecodeFloatReflection[float64](reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{fmt.Sprint(value)}),
		}, nil
	case *schema.TypeRepresentationEnum:
		value, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		if !slices.Contains(sl.OneOf, value) {
			return nil, fmt.Errorf("%s: the value must be one of %v, got %s", strings.Join(fieldPaths, ""), sl.OneOf, value)
		}

		return []ParameterItem{NewParameterItem([]Key{}, []string{value})}, nil
	case *schema.TypeRepresentationDate:
		value, err := utils.DecodeDateTimeReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{value.Format(time.DateOnly)}),
		}, nil
	case *schema.TypeRepresentationTimestamp, *schema.TypeRepresentationTimestampTZ:
		value, err := utils.DecodeDateTimeReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{
			NewParameterItem([]Key{}, []string{value.Format(time.RFC3339)}),
		}, nil
	case *schema.TypeRepresentationUUID:
		rawValue, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		_, err = uuid.Parse(rawValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}

		return []ParameterItem{NewParameterItem([]Key{}, []string{rawValue})}, nil
	default:
		return encodeParameterReflectionValues(reflectValue, fieldPaths)
	}
}

func encodeParameterReflectionValues(reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	reflectValue, ok := sdkUtils.UnwrapPointerFromReflectValue(reflectValue)
	if !ok {
		return ParameterItems{}, nil
	}

	kind := reflectValue.Kind()
	switch kind {
	case reflect.Bool:
		return []ParameterItem{
			NewParameterItem([]Key{}, []string{strconv.FormatBool(reflectValue.Bool())}),
		}, nil
	case reflect.String:
		return []ParameterItem{NewParameterItem([]Key{}, []string{reflectValue.String()})}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return []ParameterItem{
			NewParameterItem([]Key{}, []string{strconv.FormatInt(reflectValue.Int(), 10)}),
		}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []ParameterItem{
			NewParameterItem([]Key{}, []string{strconv.FormatUint(reflectValue.Uint(), 10)}),
		}, nil
	case reflect.Float32, reflect.Float64:
		return []ParameterItem{
			NewParameterItem([]Key{}, []string{fmt.Sprintf("%f", reflectValue.Float())}),
		}, nil
	case reflect.Slice, reflect.Array:
		return encodeParameterReflectionSlice(reflectValue, fieldPaths)
	case reflect.Map, reflect.Interface:
		return encodeParameterReflectionMap(reflectValue, fieldPaths)
	case reflect.Struct:
		return encodeParameterReflectionStruct(reflectValue, fieldPaths)
	default:
		return nil, fmt.Errorf("%s: failed to encode parameter, got %s", strings.Join(fieldPaths, ""), kind)
	}
}

func encodeParameterReflectionSlice(reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	results := ParameterItems{}
	valueLen := reflectValue.Len()
	for i := range valueLen {
		elem := reflectValue.Index(i)
		outputs, err := encodeParameterReflectionValues(elem, append(fieldPaths, fmt.Sprintf("[%d]", i)))
		if err != nil {
			return nil, err
		}

		for _, output := range outputs {
			results.Add(append([]Key{NewIndexKey(i)}, output.Keys()...), output.Values())
		}
	}

	return results, nil
}

func encodeParameterReflectionStruct(reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	results := ParameterItems{}
	reflectType := reflectValue.Type()
	for fieldIndex := range reflectValue.NumField() {
		fieldVal := reflectValue.Field(fieldIndex)
		fieldType := reflectType.Field(fieldIndex)
		output, err := encodeParameterReflectionValues(fieldVal, append(fieldPaths, "."+fieldType.Name))
		if err != nil {
			return nil, err
		}

		for _, pair := range output {
			results.Add(append([]Key{NewKey(fieldType.Name)}, pair.Keys()...), pair.Values())
		}
	}

	return results, nil
}

func encodeParameterReflectionMap(reflectValue reflect.Value, fieldPaths []string) (ParameterItems, error) {
	results := ParameterItems{}
	anyValue := reflectValue.Interface()
	object, ok := anyValue.(map[string]any)
	if !ok {
		b, err := json.Marshal(anyValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}
		values := []string{strings.Trim(string(b), `"`)}

		return []ParameterItem{NewParameterItem([]Key{}, values)}, nil
	}

	for key, fieldValue := range object {
		output, err := encodeParameterReflectionValues(reflect.ValueOf(fieldValue), append(fieldPaths, "."+key))
		if err != nil {
			return nil, err
		}

		for _, pair := range output {
			results.Add(append([]Key{NewKey(key)}, pair.Keys()...), pair.Values())
		}
	}

	return results, nil
}

func buildParamQueryKey(name string, encObject rest.EncodingObject, keys Keys, values []string) string {
	resultKeys := []string{}
	if name != "" {
		resultKeys = append(resultKeys, name)
	}
	keysLength := len(keys)
	// non-explode or explode form object does not require param name
	// /users?role=admin&firstName=Alex
	if (encObject.Explode != nil && !*encObject.Explode) ||
		(len(values) == 1 && encObject.Style == rest.EncodingStyleForm && (keysLength > 1 || (keysLength == 1 && !keys[0].IsEmpty()))) {
		resultKeys = []string{}
	}

	if keysLength > 0 {
		if encObject.Style != rest.EncodingStyleDeepObject && keys[keysLength-1].IsEmpty() {
			keys = keys[:keysLength-1]
		}

		for i, key := range keys {
			if len(resultKeys) == 0 {
				resultKeys = append(resultKeys, key.String())

				continue
			}
			if i == len(keys)-1 && key.Index() != nil {
				// the last element of array in the deepObject style doesn't have index
				resultKeys = append(resultKeys, "[]")

				continue
			}

			resultKeys = append(resultKeys, "["+key.String()+"]")
		}
	}

	return strings.Join(resultKeys, "")
}

func evalQueryParameterURL(q *url.Values, name string, encObject rest.EncodingObject, keys Keys, values []string) {
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

func setHeaderParameters(header *http.Header, param *rest.RequestParameter, queryParams ParameterItems) {
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
