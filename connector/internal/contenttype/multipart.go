package contenttype

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"slices"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// MultipartFormEncoder implements a multipart/form encoder.
type MultipartFormEncoder struct {
	schema       *rest.NDCHttpSchema
	paramEncoder *URLParameterEncoder
	operation    *rest.OperationInfo
	arguments    map[string]any
}

func NewMultipartFormEncoder(schema *rest.NDCHttpSchema, operation *rest.OperationInfo, arguments map[string]any) *MultipartFormEncoder {
	return &MultipartFormEncoder{
		schema:       schema,
		paramEncoder: NewURLParameterEncoder(schema, rest.ContentTypeMultipartFormData),
		operation:    operation,
		arguments:    arguments,
	}
}

// Encode the multipart form.
func (c *MultipartFormEncoder) Encode(bodyData any) ([]byte, string, error) {
	bodyInfo, ok := c.operation.Arguments[rest.BodyKey]
	if !ok {
		return nil, "", errRequestBodyTypeRequired
	}

	buffer := new(bytes.Buffer)
	writer := NewMultipartWriter(buffer)

	if err := c.evalMultipartForm(writer, &bodyInfo, reflect.ValueOf(bodyData)); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func (mfb *MultipartFormEncoder) evalMultipartForm(w *MultipartWriter, bodyInfo *rest.ArgumentInfo, bodyData reflect.Value) error {
	bodyData, ok := utils.UnwrapPointerFromReflectValue(bodyData)
	if !ok {
		return nil
	}

	switch bodyType := bodyInfo.Type.Interface().(type) {
	case *schema.NullableType:
		return mfb.evalMultipartForm(w, &rest.ArgumentInfo{
			ArgumentInfo: schema.ArgumentInfo{
				Type: bodyType.UnderlyingType,
			},
			HTTP: bodyInfo.HTTP,
		}, bodyData)
	case *schema.NamedType:
		if !ok {
			return fmt.Errorf("%s: %w", rest.BodyKey, errArgumentRequired)
		}
		bodyObject, ok := mfb.schema.ObjectTypes[bodyType.Name]
		if !ok {
			break
		}

		kind := bodyData.Kind()
		switch kind {
		case reflect.Map, reflect.Interface:
			bi := bodyData.Interface()
			bodyMap, ok := bi.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid multipart form body, expected object, got %v", bi)
			}

			for key, fieldInfo := range bodyObject.Fields {
				fieldValue := bodyMap[key]
				var enc *rest.EncodingObject
				if len(mfb.operation.Request.RequestBody.Encoding) > 0 {
					en, ok := mfb.operation.Request.RequestBody.Encoding[key]
					if ok {
						enc = &en
					}
				}

				if err := mfb.evalMultipartFieldValueRecursive(w, key, reflect.ValueOf(fieldValue), &fieldInfo, enc); err != nil {
					return err
				}
			}

			return nil
		case reflect.Struct:
			reflectType := bodyData.Type()
			for fieldIndex := range bodyData.NumField() {
				fieldValue := bodyData.Field(fieldIndex)
				fieldType := reflectType.Field(fieldIndex)
				fieldInfo, ok := bodyObject.Fields[fieldType.Name]
				if !ok {
					continue
				}

				var enc *rest.EncodingObject
				if len(mfb.operation.Request.RequestBody.Encoding) > 0 {
					en, ok := mfb.operation.Request.RequestBody.Encoding[fieldType.Name]
					if ok {
						enc = &en
					}
				}

				if err := mfb.evalMultipartFieldValueRecursive(w, fieldType.Name, fieldValue, &fieldInfo, enc); err != nil {
					return err
				}
			}

			return nil
		}
	}

	return fmt.Errorf("invalid multipart form body, expected object, got %v", bodyInfo.Type)
}

func (mfb *MultipartFormEncoder) evalMultipartFieldValueRecursive(w *MultipartWriter, name string, value reflect.Value, fieldInfo *rest.ObjectField, enc *rest.EncodingObject) error {
	underlyingValue, notNull := utils.UnwrapPointerFromReflectValue(value)
	argTypeT, err := fieldInfo.Type.InterfaceT()
	switch argType := argTypeT.(type) {
	case *schema.ArrayType:
		if !notNull {
			return fmt.Errorf("%s: %w", name, errArgumentRequired)
		}
		if enc != nil && slices.Contains(enc.ContentType, rest.ContentTypeJSON) {
			var headers http.Header
			var err error
			if len(enc.Headers) > 0 {
				headers, err = mfb.evalEncodingHeaders(enc.Headers)
				if err != nil {
					return err
				}
			}

			return w.WriteJSON(name, value.Interface(), headers)
		}

		if !slices.Contains([]reflect.Kind{reflect.Slice, reflect.Array}, value.Kind()) {
			return fmt.Errorf("%s: expected array type, got %v", name, value.Kind())
		}

		for i := range value.Len() {
			elem := value.Index(i)
			err := mfb.evalMultipartFieldValueRecursive(w, name+"[]", elem, &rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: argType.ElementType,
				},
				HTTP: fieldInfo.HTTP.Items,
			}, enc)
			if err != nil {
				return err
			}
		}

		return nil
	case *schema.NullableType:
		if !notNull {
			return nil
		}

		return mfb.evalMultipartFieldValueRecursive(w, name, underlyingValue, &rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: argType.UnderlyingType,
			},
			HTTP: fieldInfo.HTTP,
		}, enc)
	case *schema.NamedType:
		if !notNull {
			return fmt.Errorf("%s: %w", name, errArgumentRequired)
		}
		var headers http.Header
		var err error
		if enc != nil && len(enc.Headers) > 0 {
			headers, err = mfb.evalEncodingHeaders(enc.Headers)
			if err != nil {
				return err
			}
		}

		if iScalar, ok := mfb.schema.ScalarTypes[argType.Name]; ok {
			switch iScalar.Representation.Interface().(type) {
			case *schema.TypeRepresentationBytes:
				return w.WriteDataURI(name, value.Interface(), headers)
			default:
			}
		}

		if enc != nil && slices.Contains(enc.ContentType, rest.ContentTypeJSON) {
			return w.WriteJSON(name, value, headers)
		}

		params, err := mfb.paramEncoder.EncodeParameterValues(fieldInfo, value, []string{})
		if err != nil {
			return err
		}

		if len(params) == 0 {
			return nil
		}

		for _, p := range params {
			keys := p.Keys()
			values := p.Values()
			fieldName := name

			if len(keys) > 0 {
				keys = append([]Key{NewKey(name)}, keys...)
				fieldName = keys.String()
			}

			if len(values) > 1 {
				fieldName += "[]"
				for _, v := range values {
					if err = w.WriteField(fieldName, v, headers); err != nil {
						return err
					}
				}
			} else if len(values) == 1 {
				if err = w.WriteField(fieldName, values[0], headers); err != nil {
					return err
				}
			}
		}

		return nil
	case *schema.PredicateType:
		return fmt.Errorf("%s: predicate type is not supported", name)
	default:
		return fmt.Errorf("%s: %w", name, err)
	}
}

func (mfb *MultipartFormEncoder) evalEncodingHeaders(encHeaders map[string]rest.RequestParameter) (http.Header, error) {
	results := http.Header{}
	for key, param := range encHeaders {
		argumentName := param.ArgumentName
		if argumentName == "" {
			argumentName = key
		}
		argumentInfo, ok := mfb.operation.Arguments[argumentName]
		if !ok {
			continue
		}
		rawHeaderValue, ok := mfb.arguments[argumentName]
		if !ok {
			continue
		}

		headerParams, err := mfb.paramEncoder.EncodeParameterValues(&rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: argumentInfo.Type,
			},
			HTTP: param.Schema,
		}, reflect.ValueOf(rawHeaderValue), []string{})
		if err != nil {
			return nil, err
		}

		param.Name = key
		SetHeaderParameters(&results, &param, headerParams)
	}

	return results, nil
}

// EncodeArbitrary encodes the unknown data to multipart/form.
func (c *MultipartFormEncoder) EncodeArbitrary(bodyData any) ([]byte, string, error) {
	buffer := new(bytes.Buffer)
	writer := NewMultipartWriter(buffer)

	reflectValue, ok := utils.UnwrapPointerFromReflectValue(reflect.ValueOf(bodyData))
	if ok {
		valueMap, ok := reflectValue.Interface().(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("invalid body for multipart/form, expected object, got: %s", reflectValue.Kind())
		}

		for key, value := range valueMap {
			if err := c.evalFormDataReflection(writer, key, reflect.ValueOf(value)); err != nil {
				return nil, "", fmt.Errorf("invalid body for multipart/form, %s: %w", key, err)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func (c *MultipartFormEncoder) evalFormDataReflection(w *MultipartWriter, key string, reflectValue reflect.Value) error {
	reflectValue, ok := utils.UnwrapPointerFromReflectValue(reflectValue)
	if !ok {
		return nil
	}

	kind := reflectValue.Kind()
	switch kind {
	case reflect.Map, reflect.Struct, reflect.Array, reflect.Slice:
		return w.WriteJSON(key, reflectValue.Interface(), http.Header{})
	default:
		value, err := StringifySimpleScalar(reflectValue, kind)
		if err != nil {
			return err
		}

		return w.WriteField(key, value, http.Header{})
	}
}
