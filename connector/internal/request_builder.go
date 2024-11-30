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

	"github.com/hasura/ndc-http/connector/internal/contenttype"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// RequestBuilder builds requests to the remote service
type RequestBuilder struct {
	Schema    *rest.NDCHttpSchema
	Operation *rest.OperationInfo
	Arguments map[string]any
	Runtime   rest.RuntimeSettings
}

// NewRequestBuilder creates a new RequestBuilder instance
func NewRequestBuilder(restSchema *rest.NDCHttpSchema, operation *rest.OperationInfo, arguments map[string]any, runtime rest.RuntimeSettings) *RequestBuilder {
	return &RequestBuilder{
		Schema:    restSchema,
		Operation: operation,
		Arguments: arguments,
		Runtime:   runtime,
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

	rawRequest := c.Operation.Request

	request := &RetryableRequest{
		URL:        *endpoint,
		RawRequest: rawRequest,
		Headers:    headers,
		Runtime:    c.Runtime,
	}

	if err := c.buildRequestBody(request, rawRequest); err != nil {
		return nil, err
	}

	if rawRequest.RuntimeSettings != nil {
		if rawRequest.RuntimeSettings.Timeout > 0 {
			request.Runtime.Timeout = rawRequest.RuntimeSettings.Timeout
		}
		if rawRequest.RuntimeSettings.Retry.Times > 0 {
			request.Runtime.Retry.Times = rawRequest.RuntimeSettings.Retry.Times
		}
		if rawRequest.RuntimeSettings.Retry.Delay > 0 {
			request.Runtime.Retry.Delay = rawRequest.RuntimeSettings.Retry.Delay
		}
		if rawRequest.RuntimeSettings.Retry.HTTPStatus != nil {
			request.Runtime.Retry.HTTPStatus = rawRequest.RuntimeSettings.Retry.HTTPStatus
		}
	}
	if request.Runtime.Retry.HTTPStatus == nil {
		request.Runtime.Retry.HTTPStatus = defaultRetryHTTPStatus
	}

	return request, nil
}

func (c *RequestBuilder) buildRequestBody(request *RetryableRequest, rawRequest *rest.Request) error {
	if rawRequest.RequestBody == nil {
		request.ContentType = rest.ContentTypeJSON

		return nil
	}

	contentType := parseContentType(rawRequest.RequestBody.ContentType)
	request.ContentType = rawRequest.RequestBody.ContentType
	bodyInfo, infoOk := c.Operation.Arguments[rest.BodyKey]
	bodyData, ok := c.Arguments[rest.BodyKey]

	if ok && bodyData != nil {
		binaryBody := c.getRequestUploadBody(c.Operation.Request, &bodyInfo)

		switch {
		case binaryBody != nil:
			b64, err := utils.DecodeString(bodyData)
			if err != nil {
				return err
			}
			dataURI, err := contenttype.DecodeDataURI(b64)
			if err != nil {
				return err
			}
			r := bytes.NewReader([]byte(dataURI.Data))
			request.ContentLength = r.Size()
			request.Body = r

			return nil
		case strings.HasPrefix(contentType, "text/"):
			r := bytes.NewReader([]byte(fmt.Sprint(bodyData)))
			request.ContentLength = r.Size()
			request.Body = r

			return nil
		case strings.HasPrefix(contentType, "multipart/"):
			r, contentType, err := c.createMultipartForm(bodyData)
			if err != nil {
				return err
			}

			request.ContentType = contentType
			request.ContentLength = r.Size()
			request.Body = r

			return nil
		case contentType == rest.ContentTypeFormURLEncoded:
			r, err := c.createFormURLEncoded(&bodyInfo, bodyData)
			if err != nil {
				return err
			}
			request.Body = r

			return nil
		case contentType == rest.ContentTypeJSON || contentType == "":

			bodyBytes, err := json.Marshal(bodyData)
			if err != nil {
				return err
			}

			request.ContentLength = int64(len(bodyBytes))
			request.Body = bytes.NewReader(bodyBytes)

			return nil
		case contentType == rest.ContentTypeXML:
			bodyBytes, err := contenttype.NewXMLEncoder(c.Schema).Encode(&bodyInfo, bodyData)
			if err != nil {
				return err
			}

			request.ContentLength = int64(len(bodyBytes))
			request.Body = bytes.NewReader(bodyBytes)

			return nil
		default:
			return fmt.Errorf("unsupported content type %s", contentType)
		}
	} else if infoOk {
		ty, err := bodyInfo.Type.Type()
		if err != nil {
			return err
		}
		if ty != schema.TypeNullable {
			return errRequestBodyRequired
		}
	}

	return nil
}

func (c *RequestBuilder) createFormURLEncoded(bodyInfo *rest.ArgumentInfo, bodyData any) (io.ReadSeeker, error) {
	queryParams, err := c.encodeParameterValues(&rest.ObjectField{
		ObjectField: schema.ObjectField{
			Type: bodyInfo.Type,
		},
		HTTP: bodyInfo.HTTP.Schema,
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
		evalQueryParameterURL(&q, "", bodyInfo.HTTP.EncodingObject, keys, qp.Values())
	}
	rawQuery := encodeQueryValues(q, true)

	return bytes.NewReader([]byte(rawQuery)), nil
}

func (c *RequestBuilder) createMultipartForm(bodyData any) (*bytes.Reader, string, error) {
	bodyInfo, ok := c.Operation.Arguments[rest.BodyKey]
	if !ok {
		return nil, "", errRequestBodyTypeRequired
	}

	buffer := new(bytes.Buffer)
	writer := contenttype.NewMultipartWriter(buffer)

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

func (c *RequestBuilder) evalMultipartForm(w *contenttype.MultipartWriter, bodyInfo *rest.ArgumentInfo, bodyData reflect.Value) error {
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
			HTTP: bodyInfo.HTTP,
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

func (c *RequestBuilder) evalMultipartFieldValueRecursive(w *contenttype.MultipartWriter, name string, value reflect.Value, fieldInfo *rest.ObjectField, enc *rest.EncodingObject) error {
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

		return c.evalMultipartFieldValueRecursive(w, name, underlyingValue, &rest.ObjectField{
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
			headers, err = c.evalEncodingHeaders(enc.Headers)
			if err != nil {
				return err
			}
		}

		if iScalar, ok := c.Schema.ScalarTypes[argType.Name]; ok {
			switch iScalar.Representation.Interface().(type) {
			case *schema.TypeRepresentationBytes:
				return w.WriteDataURI(name, value.Interface(), headers)
			default:
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
			HTTP: param.Schema,
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
	if rawRequest.RequestBody.ContentType == rest.ContentTypeOctetStream {
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
