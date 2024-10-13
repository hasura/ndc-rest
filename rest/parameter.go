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

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/rest/internal"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	sdkUtils "github.com/hasura/ndc-sdk-go/utils"
)

var urlAndHeaderLocations = []rest.ParameterLocation{rest.InPath, rest.InQuery, rest.InPath}

func (c *RESTConnector) evalURLAndHeaderParameters(request *rest.Request, argumentsSchema map[string]rest.ArgumentInfo, arguments map[string]any) (string, http.Header, error) {
	endpoint, err := url.Parse(request.URL)
	if err != nil {
		return "", nil, err
	}
	headers := http.Header{}
	for k, h := range request.Headers {
		v := h.Value()
		if v != nil && *v != "" {
			headers.Add(k, *v)
		}
	}

	for argumentKey, argumentInfo := range argumentsSchema {
		if argumentInfo.Rest == nil || !slices.Contains(urlAndHeaderLocations, argumentInfo.Rest.In) {
			continue
		}
		if err := c.evalURLAndHeaderParameterBySchema(endpoint, &headers, &argumentInfo, arguments[argumentKey]); err != nil {
			return "", nil, fmt.Errorf("%s: %w", argumentKey, err)
		}
	}
	return endpoint.String(), headers, nil
}

// the query parameters serialization follows [OAS 3.1 spec]
//
// [OAS 3.1 spec]: https://swagger.io/docs/specification/serialization/
func (c *RESTConnector) evalURLAndHeaderParameterBySchema(endpoint *url.URL, header *http.Header, argumentInfo *rest.ArgumentInfo, value any) error {
	queryParams, err := c.encodeParameterValues(&rest.ObjectField{
		ObjectField: schema.ObjectField{
			Type: argumentInfo.Type,
		},
		Rest: argumentInfo.Rest.Schema,
	}, reflect.ValueOf(value), []string{argumentInfo.Rest.Name})
	if err != nil {
		return err
	}

	if len(queryParams) == 0 {
		return nil
	}

	// following the OAS spec to serialize parameters
	// https://swagger.io/docs/specification/serialization/
	// https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md#parameter-object
	switch argumentInfo.Rest.In {
	case rest.InHeader:
		setHeaderParameters(header, argumentInfo.Rest, queryParams)
	case rest.InQuery:
		q := endpoint.Query()
		for _, qp := range queryParams {
			evalQueryParameterURL(&q, argumentInfo.Rest.Name, argumentInfo.Rest.EncodingObject, qp.Keys(), qp.Values())
		}
		endpoint.RawQuery = encodeQueryValues(q, argumentInfo.Rest.AllowReserved)
	case rest.InPath:
		defaultParam := queryParams.FindDefault()
		if defaultParam != nil {
			endpoint.Path = strings.ReplaceAll(endpoint.Path, "{"+argumentInfo.Rest.Name+"}", strings.Join(defaultParam.Values(), ","))
		}
	}
	return nil
}

func (c *RESTConnector) encodeParameterValues(objectField *rest.ObjectField, reflectValue reflect.Value, fieldPaths []string) (internal.ParameterItems, error) {
	results := internal.ParameterItems{}

	if reflectValue.Kind() == reflect.Invalid {
		return results, nil
	}
	typeSchema := objectField.Rest
	var typeName string
	if len(typeSchema.Type) > 0 {
		typeName = strings.ToLower(typeSchema.Type[0])
	}
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
			Rest: typeSchema,
		}, reflectValue, fieldPaths)
	case *schema.ArrayType:
		if !slices.Contains([]reflect.Kind{reflect.Slice, reflect.Array}, reflectValue.Kind()) {
			return nil, fmt.Errorf("%s: expected array, got %v", strings.Join(fieldPaths, ""), reflectValue.Interface())
		}
		for i := range reflectValue.Len() {
			propPaths := append(fieldPaths, "["+strconv.Itoa(i)+"]")

			outputs, err := c.encodeParameterValues(&rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: ty.ElementType,
				},
				Rest: typeSchema.Items,
			}, reflectValue.Index(i), propPaths)
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
	case *schema.NamedType:
		iScalar, ok := c.schema.ScalarTypes[typeName]
		if ok {
			return encodeParameterReflectionValues(reflectValue, &iScalar, fieldPaths)
		}
	default:
		return nil, fmt.Errorf("invalid type %v", objectField.Type)
	}
	// switch typeName {
	// case "object":
	// 	objectInfo, ok := c.schema.ObjectTypes
	// 	if len(typeSchema.Properties) == 0 {
	// 		return results, nil
	// 	}
	// 	mapValue, ok := value.(map[string]any)
	// 	if !ok {
	// 		return nil, fmt.Errorf("%s: expected object, got %v", strings.Join(fieldPaths, ""), value)
	// 	}

	// 	for k, prop := range typeSchema.Properties {
	// 		propPaths := append(fieldPaths, "."+k)
	// 		fieldVal, ok := mapValue[k]
	// 		if !ok {
	// 			if !prop.Nullable {
	// 				return nil, fmt.Errorf("parameter %s is required", strings.Join(propPaths, ""))
	// 			}
	// 			continue
	// 		}

	// 		output, err := c.encodeParameterValues(&prop, fieldVal, propPaths)
	// 		if err != nil {
	// 			return nil, err
	// 		}

	// 		for _, pair := range output {
	// 			results.Add(append([]internal.Key{internal.NewKey(k)}, pair.Keys()...), pair.Values())
	// 		}
	// 	}

	// 	return results, nil
	// }
}

func encodeParameterReflectionValues(reflectValue reflect.Value, scalar *schema.ScalarType, fieldPaths []string) (internal.ParameterItems, error) {
	results := internal.ParameterItems{}

	switch sl := scalar.Representation.Interface().(type) {
	case *schema.TypeRepresentationBoolean:
		value, err := utils.DecodeBooleanReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}
		return []internal.ParameterItem{
			internal.NewParameterItem([]internal.Key{}, []string{strconv.FormatBool(value)}),
		}, nil
	case *schema.TypeRepresentationString:
		value, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}
		return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, []string{value})}, nil
	case *schema.TypeRepresentationInteger, *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64, *schema.TypeRepresentationBigInteger:
		value, err := utils.DecodeIntReflection[int64](reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}
		return []internal.ParameterItem{
			internal.NewParameterItem([]internal.Key{}, []string{strconv.FormatInt(value, 10)}),
		}, nil
	case *schema.TypeRepresentationNumber, *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64, *schema.TypeRepresentationBigDecimal:
		value, err := utils.DecodeFloatReflection[float64](reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}
		return []internal.ParameterItem{
			internal.NewParameterItem([]internal.Key{}, []string{fmt.Sprint(value)}),
		}, nil
	case *schema.TypeRepresentationEnum:
		value, err := utils.DecodeStringReflection(reflectValue)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}
		if !slices.Contains(sl.OneOf, value) {
			return nil, fmt.Errorf("%s: the value must be one of %v, got %s", strings.Join(fieldPaths, ""), sl.OneOf, value)
		}
		return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, []string{value})}, nil
	default:
		b, err := json.Marshal(reflectValue.Interface())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
		}
		values := []string{strings.Trim(string(b), `"`)}
		return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, values)}, nil
	}

	kind := reflectValue.Kind()
	switch kind {
	case reflect.Bool:
		// if strings.Contains([]schema.ScalarType{schema.TypeRepresentationBoolean, schema.TypeRepresentationJSON}, scalar.Representation.Type())  ==
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
		for i := range valueLen {
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
			propPaths := append(fieldPaths, "."+key)
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
			return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
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
		return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
	}

	return []internal.ParameterItem{
		internal.NewParameterItem([]internal.Key{}, []string{strconv.FormatBool(result)}),
	}, nil
}

func encodeParameterString(value any, fieldPaths []string) (internal.ParameterItems, error) {
	result, err := sdkUtils.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
	}
	return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, []string{result})}, nil
}

func encodeParameterInt(value any, fieldPaths []string) (internal.ParameterItems, error) {
	intValue, err := sdkUtils.DecodeInt[int64](value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
	}
	return []internal.ParameterItem{internal.NewParameterItem([]internal.Key{}, []string{strconv.FormatInt(intValue, 10)})}, nil
}

func encodeParameterFloat(value any, fieldPaths []string) (internal.ParameterItems, error) {
	floatValue, err := sdkUtils.DecodeFloat[float64](value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
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
