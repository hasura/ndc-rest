package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/rest/internal"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
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
		argSchema, schemaOk := argumentsSchema[param.Name]
		value, ok := arguments[param.Name]

		if !schemaOk || !ok || utils.IsNil(value) {
			if param.Schema != nil && !param.Schema.Nullable {
				return "", nil, fmt.Errorf("parameter %s is required", param.Name)
			}
		} else if err := c.evalURLAndHeaderParameterBySchema(endpoint, &headers, &param, argSchema.Type, value); err != nil {
			return "", nil, err
		}
	}
	return endpoint.String(), headers, nil
}

// the query parameters serialization follows [OAS 3.1 spec]
//
// [OAS 3.1 spec]: https://swagger.io/docs/specification/serialization/
func (c *RESTConnector) evalURLAndHeaderParameterBySchema(endpoint *url.URL, header *http.Header, param *rest.RequestParameter, argumentType schema.Type, value any) error {
	if utils.IsNil(value) {
		return nil
	}

	queryParams, err := c.encodeParameterValues(param, argumentType, value)
	if err != nil || len(queryParams) == 0 {
		return err
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

func (c *RESTConnector) encodeParameterValues(param *rest.RequestParameter, argumentType schema.Type, value any) (internal.StringSlicePairs, error) {

	switch arg := argumentType.Interface().(type) {
	case *schema.NamedType:
		var val string
		iScalar, ok := c.schema.ScalarTypes[arg.Name]
		if ok {
			switch ty := iScalar.Representation.Interface().(type) {
			case *schema.TypeRepresentationBoolean:
				return []*internal.StringSlicePair{
					internal.NewStringSlicePair([]string{}, []string{fmt.Sprintf("%t", value)}),
				}, nil
			case *schema.TypeRepresentationEnum:
				val = fmt.Sprint(value)
				if !slices.Contains(ty.OneOf, val) {
					return nil, fmt.Errorf("%s: invalid enum value '%s'", param.Name, value)
				}

				return []*internal.StringSlicePair{internal.NewStringSlicePair([]string{}, []string{fmt.Sprint(value)})}, nil
			default:
				return []*internal.StringSlicePair{
					internal.NewStringSlicePair([]string{}, []string{fmt.Sprint(value)}),
				}, nil
			}
		}

		results := []*internal.StringSlicePair{}
		object, ok := c.schema.ObjectTypes[arg.Name]
		if ok {
			mapValue, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("cannot evaluate object parameter of %s, got: %+v", param.Name, reflect.TypeOf(value).Name())
			}

			for k, v := range object.Fields {
				fieldVal, ok := mapValue[k]
				if !ok || utils.IsNil(fieldVal) {
					continue
				}

				output, err := c.encodeParameterValues(param, v.Type, fieldVal)
				if err != nil {
					return nil, err
				}
				for _, pair := range output {
					results = append(results, internal.NewStringSlicePair(append([]string{k}, pair.Keys()...), pair.Values()))
				}
			}
			return results, nil
		}

		b, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		values := []string{strings.Trim(string(b), `"`)}
		return []*internal.StringSlicePair{internal.NewStringSlicePair([]string{}, values)}, nil
	case *schema.NullableType:
		return c.encodeParameterValues(param, arg.UnderlyingType, value)
	case *schema.ArrayType:
		if !slices.Contains([]rest.ParameterLocation{rest.InHeader, rest.InQuery}, param.In) {
			return nil, fmt.Errorf("cannot evaluate array parameter to %s", param.In)
		}
		if utils.IsNil(value) {
			return []*internal.StringSlicePair{}, nil
		}
		arrayValue, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("cannot evaluate array parameter, expected array, got: %+v", reflect.TypeOf(value).Name())
		}

		var results internal.StringSlicePairs
		for _, item := range arrayValue {
			outputs, err := c.encodeParameterValues(param, arg.ElementType, item)
			if err != nil {
				return nil, err
			}

			for _, output := range outputs {
				results.Add(append([]string{""}, output.Keys()...), output.Values())
			}
		}
		return results, nil
	}

	return nil, nil
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
