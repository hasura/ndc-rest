package internal

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/hasura/ndc-http/connector/internal/contenttype"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
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
