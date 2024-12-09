package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
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
			r, contentType, err := contenttype.NewMultipartFormEncoder(c.Schema, c.Operation, c.Arguments).Encode(bodyData)
			if err != nil {
				return err
			}

			request.ContentType = contentType
			request.ContentLength = r.Size()
			request.Body = r

			return nil
		case contentType == rest.ContentTypeFormURLEncoded:
			r, err := contenttype.NewURLParameterEncoder(c.Schema).Encode(&bodyInfo, bodyData)
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

func (c *RequestBuilder) getRequestUploadBody(rawRequest *rest.Request, bodyInfo *rest.ArgumentInfo) *rest.RequestBody {
	if rawRequest.RequestBody == nil || bodyInfo == nil {
		return nil
	}
	if rawRequest.RequestBody.ContentType == rest.ContentTypeOctetStream {
		return rawRequest.RequestBody
	}

	bi, ok, err := contenttype.UnwrapNullableType(bodyInfo.Type)
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
	queryParams, err := contenttype.NewURLParameterEncoder(c.Schema).EncodeParameterValues(&rest.ObjectField{
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
		contenttype.SetHeaderParameters(header, argumentInfo.HTTP, queryParams)
	case rest.InQuery:
		q := endpoint.Query()
		for _, qp := range queryParams {
			contenttype.EvalQueryParameterURL(&q, argumentKey, argumentInfo.HTTP.EncodingObject, qp.Keys(), qp.Values())
		}
		endpoint.RawQuery = contenttype.EncodeQueryValues(q, argumentInfo.HTTP.AllowReserved)
	case rest.InPath:
		defaultParam := queryParams.FindDefault()
		if defaultParam != nil {
			endpoint.Path = strings.ReplaceAll(endpoint.Path, "{"+argumentKey+"}", strings.Join(defaultParam.Values(), ","))
		}
	}

	return nil
}
