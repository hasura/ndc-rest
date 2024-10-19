package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// RequestBuilder builds requests to the remote service
type RequestBuilder struct {
	Schema    *rest.NDCRestSchema
	Operation *rest.OperationInfo
	Arguments map[string]any
}

// NewRequestBuilder creates a new RequestBuilder instance
func NewRequestBuilder(restSchema *rest.NDCRestSchema, operation *rest.OperationInfo, arguments map[string]any) *RequestBuilder {
	return &RequestBuilder{
		Schema:    restSchema,
		Operation: operation,
		Arguments: arguments,
	}
}

// Build evaluates and builds a RetryableRequest
func (c *RequestBuilder) Build() (*RetryableRequest, error) {
	endpoint, headers, err := c.evalURLAndHeaderParameters()
	if err != nil {
		return nil, schema.UnprocessableContentError("failed to evaluate URL and Headers from parameters", map[string]any{
			"cause": err.Error(),
		})
	}

	var buffer io.ReadSeeker

	rawRequest := c.Operation.Request
	contentType := rest.ContentTypeJSON

	if rawRequest.RequestBody != nil {
		contentType = rawRequest.RequestBody.ContentType
		bodyInfo, infoOk := c.Operation.Arguments[rest.BodyKey]
		bodyData, ok := c.Arguments[rest.BodyKey]
		if ok && bodyData != nil {
			var err error
			binaryBody := c.getRequestUploadBody(c.Operation.Request, &bodyInfo)
			if binaryBody != nil {
				b64, err := utils.DecodeString(bodyData)
				if err != nil {
					return nil, err
				}
				dataURI, err := DecodeDataURI(b64)
				if err != nil {
					return nil, err
				}
				buffer = bytes.NewReader([]byte(dataURI.Data))
			} else if strings.HasPrefix(contentType, "text/") {
				buffer = bytes.NewReader([]byte(fmt.Sprint(bodyData)))
			} else if strings.HasPrefix(contentType, "multipart/") {
				buffer, contentType, err = c.createMultipartForm(bodyData)
				if err != nil {
					return nil, err
				}
			} else {
				switch contentType {
				case rest.ContentTypeFormURLEncoded:
					buffer, err = c.createFormURLEncoded(&bodyInfo, bodyData)
					if err != nil {
						return nil, err
					}
				case rest.ContentTypeJSON, "":
					bodyBytes, err := json.Marshal(bodyData)
					if err != nil {
						return nil, err
					}

					buffer = bytes.NewReader(bodyBytes)
				default:
					return nil, fmt.Errorf("unsupported content type %s", contentType)
				}
			}
		} else if infoOk {
			ty, err := bodyInfo.Type.Type()
			if err != nil {
				return nil, err
			}
			if ty != schema.TypeNullable {
				return nil, errRequestBodyRequired
			}
		}
	}

	request := &RetryableRequest{
		URL:         endpoint,
		RawRequest:  rawRequest,
		ContentType: contentType,
		Headers:     headers,
		Body:        buffer,
		Timeout:     rawRequest.Timeout,
		Retry:       rawRequest.Retry,
	}

	return request, nil
}

func (c *RequestBuilder) createFormURLEncoded(bodyInfo *rest.ArgumentInfo, bodyData any) (io.ReadSeeker, error) {
	queryParams, err := c.encodeParameterValues(&rest.ObjectField{
		ObjectField: schema.ObjectField{
			Type: bodyInfo.Type,
		},
		Rest: bodyInfo.Rest.Schema,
	}, reflect.ValueOf(bodyData), []string{"body"})
	if err != nil {
		return nil, err
	}

	if len(queryParams) == 0 {
		return nil, nil
	}
	q := url.Values{}
	for _, qp := range queryParams {
		keys := qp.Keys()
		evalQueryParameterURL(&q, "", bodyInfo.Rest.EncodingObject, keys, qp.Values())
	}
	rawQuery := encodeQueryValues(q, true)

	return bytes.NewReader([]byte(rawQuery)), nil
}

func (c *RequestBuilder) createMultipartForm(bodyData any) (io.ReadSeeker, string, error) {
	bodyInfo, ok := c.Operation.Arguments[rest.BodyKey]
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

	reader := bytes.NewReader(buffer.Bytes())
	buffer.Reset()

	return reader, writer.FormDataContentType(), nil
}

func (c *RequestBuilder) evalMultipartForm(w *MultipartWriter, bodyInfo *rest.ArgumentInfo, bodyData reflect.Value) error {
	bodyData, ok := utils.UnwrapPointerFromReflectValue(bodyData)
	if !ok {
		return nil
	}
	switch bodyType := bodyInfo.Type.Interface().(type) {
	case *schema.NullableType:
		return c.evalMultipartForm(w, &rest.ArgumentInfo{
			ArgumentInfo: schema.ArgumentInfo{
				Type: bodyType.UnderlyingType,
			},
			Rest: bodyInfo.Rest,
		}, bodyData)
	case *schema.NamedType:
		if !ok {
			return fmt.Errorf("%s: %w", rest.BodyKey, errArgumentRequired)
		}
		bodyObject, ok := c.Schema.ObjectTypes[bodyType.Name]
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
				if len(c.Operation.Request.RequestBody.Encoding) > 0 {
					en, ok := c.Operation.Request.RequestBody.Encoding[key]
					if ok {
						enc = &en
					}
				}

				if err := c.evalMultipartFieldValueRecursive(w, key, reflect.ValueOf(fieldValue), &fieldInfo, enc); err != nil {
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
				if len(c.Operation.Request.RequestBody.Encoding) > 0 {
					en, ok := c.Operation.Request.RequestBody.Encoding[fieldType.Name]
					if ok {
						enc = &en
					}
				}

				if err := c.evalMultipartFieldValueRecursive(w, fieldType.Name, fieldValue, &fieldInfo, enc); err != nil {
					return err
				}
			}
			return nil
		}
	}

	return fmt.Errorf("invalid multipart form body, expected object, got %v", bodyInfo.Type)
}

func (c *RequestBuilder) evalMultipartFieldValueRecursive(w *MultipartWriter, name string, value reflect.Value, fieldInfo *rest.ObjectField, enc *rest.EncodingObject) error {
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
				headers, err = c.evalEncodingHeaders(enc.Headers)
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
			err := c.evalMultipartFieldValueRecursive(w, name+"[]", elem, &rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: argType.ElementType,
				},
				Rest: fieldInfo.Rest.Items,
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
		return c.evalMultipartFieldValueRecursive(w, name, underlyingValue, &rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: argType.UnderlyingType,
			},
			Rest: fieldInfo.Rest,
		}, enc)
	case *schema.NamedType:
		if !notNull {
			return fmt.Errorf("%s: %w", name, errArgumentRequired)
		}
		var headers http.Header
		var err error
		if enc != nil && len(enc.Headers) > 0 {
			headers, err = c.evalEncodingHeaders(enc.Headers)
			if err != nil {
				return err
			}
		}
		if iScalar, ok := c.Schema.ScalarTypes[argType.Name]; ok {
			switch iScalar.Representation.Interface().(type) {
			case *schema.TypeRepresentationBytes:
				return w.WriteDataURI(name, value.Interface(), headers)
			}
		}

		if enc != nil && slices.Contains(enc.ContentType, rest.ContentTypeJSON) {
			return w.WriteJSON(name, value, headers)
		}

		params, err := c.encodeParameterValues(fieldInfo, value, []string{})
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

func (c *RequestBuilder) evalEncodingHeaders(encHeaders map[string]rest.RequestParameter) (http.Header, error) {
	results := http.Header{}
	for key, param := range encHeaders {
		argumentName := param.ArgumentName
		if argumentName == "" {
			argumentName = key
		}
		argumentInfo, ok := c.Operation.Arguments[argumentName]
		if !ok {
			continue
		}
		rawHeaderValue, ok := c.Arguments[argumentName]
		if !ok {
			continue
		}

		headerParams, err := c.encodeParameterValues(&rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: argumentInfo.Type,
			},
			Rest: param.Schema,
		}, reflect.ValueOf(rawHeaderValue), []string{})
		if err != nil {
			return nil, err
		}

		param.Name = key
		setHeaderParameters(&results, &param, headerParams)
	}

	return results, nil
}

func (c *RequestBuilder) getRequestUploadBody(rawRequest *rest.Request, bodyInfo *rest.ArgumentInfo) *rest.RequestBody {
	if rawRequest.RequestBody == nil || bodyInfo == nil {
		return nil
	}
	if rawRequest.RequestBody.ContentType == "application/octet-stream" {
		return rawRequest.RequestBody
	}

	bi, ok, err := UnwrapNullableType(bodyInfo.Type)
	if err != nil || !ok {
		return nil
	}
	namedType, ok := bi.(*schema.NamedType)
	if !ok {
		return nil
	}
	iScalar, ok := c.Schema.ScalarTypes[namedType.Name]
	if !ok {
		return nil
	}
	_, err = iScalar.Representation.AsBytes()
	if err != nil {
		return nil
	}

	return rawRequest.RequestBody
}
