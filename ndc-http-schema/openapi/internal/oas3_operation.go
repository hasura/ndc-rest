package internal

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

type oas3OperationBuilder struct {
	Arguments map[string]rest.ArgumentInfo

	builder      *OAS3Builder
	pathKey      string
	method       string
	commonParams []*v3.Parameter
}

func newOAS3OperationBuilder(builder *OAS3Builder, pathKey string, method string, commonParams []*v3.Parameter) *oas3OperationBuilder {
	return &oas3OperationBuilder{
		builder:      builder,
		pathKey:      pathKey,
		method:       method,
		commonParams: commonParams,
		Arguments:    make(map[string]rest.ArgumentInfo),
	}
}

// BuildFunction build a HTTP NDC function information from OpenAPI v3 operation
func (oc *oas3OperationBuilder) BuildFunction(itemGet *v3.Operation) (*rest.OperationInfo, string, error) {
	if oc.builder.ConvertOptions.NoDeprecation && itemGet.Deprecated != nil && *itemGet.Deprecated {
		return nil, "", nil
	}

	start := time.Now()
	funcName := buildUniqueOperationName(oc.builder.schema, itemGet.OperationId, oc.pathKey, oc.method, oc.builder.ConvertOptions)

	defer func() {
		oc.builder.Logger.Info("function",
			slog.String("name", funcName),
			slog.String("path", oc.pathKey),
			slog.String("method", oc.method),
			slog.Duration("duration", time.Since(start)),
		)
	}()

	resultType, schemaResponse, err := oc.convertResponse(itemGet.Responses, oc.pathKey, []string{funcName, "Result"})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", oc.pathKey, err)
	}

	if resultType == nil {
		return nil, "", nil
	}

	err = oc.convertParameters(itemGet.Parameters, oc.pathKey, []string{funcName})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", funcName, err)
	}

	description := oc.getOperationDescription(itemGet)
	requestURL, arguments, err := evalOperationPath(oc.builder.schema, oc.pathKey, oc.Arguments)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", funcName, err)
	}

	function := rest.OperationInfo{
		Request: &rest.Request{
			URL:      requestURL,
			Method:   "get",
			Security: convertSecurities(itemGet.Security),
			Servers:  oc.builder.convertServers(itemGet.Servers),
			Response: *schemaResponse,
		},
		Description: &description,
		Arguments:   arguments,
		ResultType:  resultType.Encode(),
	}

	return &function, funcName, nil
}

func (oc *oas3OperationBuilder) BuildProcedure(operation *v3.Operation) (*rest.OperationInfo, string, error) {
	if operation == nil || (oc.builder.ConvertOptions.NoDeprecation && operation.Deprecated != nil && *operation.Deprecated) {
		return nil, "", nil
	}

	start := time.Now()
	procName := buildUniqueOperationName(oc.builder.schema, operation.OperationId, oc.pathKey, oc.method, oc.builder.ConvertOptions)

	defer func() {
		oc.builder.Logger.Info("procedure",
			slog.String("name", procName),
			slog.String("path", oc.pathKey),
			slog.String("method", oc.method),
			slog.Duration("duration", time.Since(start)),
		)
	}()

	resultType, schemaResponse, err := oc.convertResponse(operation.Responses, oc.pathKey, []string{procName, "Result"})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", oc.pathKey, err)
	}

	if resultType == nil {
		return nil, "", nil
	}

	err = oc.convertParameters(operation.Parameters, oc.pathKey, []string{procName})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", oc.pathKey, err)
	}

	reqBody, schemaType, err := oc.convertRequestBody(operation.RequestBody, oc.pathKey, []string{procName, "Body"})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", oc.pathKey, err)
	}
	if reqBody != nil {
		description := fmt.Sprintf("Request body of %s %s", strings.ToUpper(oc.method), oc.pathKey)
		// renaming query parameter name `body` if exist to avoid conflicts
		if paramData, ok := oc.Arguments[rest.BodyKey]; ok {
			oc.Arguments["paramBody"] = paramData
		}

		oc.Arguments[rest.BodyKey] = rest.ArgumentInfo{
			ArgumentInfo: schema.ArgumentInfo{
				Description: &description,
				Type:        schemaType.Encode(),
			},
			HTTP: &rest.RequestParameter{
				In: rest.InBody,
			},
		}
	}

	description := oc.getOperationDescription(operation)
	requestURL, arguments, err := evalOperationPath(oc.builder.schema, oc.pathKey, oc.Arguments)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", procName, err)
	}

	procedure := rest.OperationInfo{
		Request: &rest.Request{
			URL:         requestURL,
			Method:      oc.method,
			Security:    convertSecurities(operation.Security),
			Servers:     oc.builder.convertServers(operation.Servers),
			RequestBody: reqBody,
			Response:    *schemaResponse,
		},
		Description: &description,
		Arguments:   arguments,
		ResultType:  resultType.Encode(),
	}

	return &procedure, procName, nil
}

func (oc *oas3OperationBuilder) convertParameters(params []*v3.Parameter, apiPath string, fieldPaths []string) error {
	if len(params) == 0 && len(oc.commonParams) == 0 {
		return nil
	}

	for _, param := range append(params, oc.commonParams...) {
		if param == nil || (param.Deprecated && oc.builder.ConvertOptions.NoDeprecation) {
			continue
		}
		paramName := param.Name
		if paramName == "" {
			return errParameterNameRequired
		}
		paramRequired := false
		if param.Required != nil && *param.Required {
			paramRequired = true
		}
		schemaType, apiSchema, err := newOAS3SchemaBuilder(oc.builder, apiPath, rest.ParameterLocation(param.In), true).
			getSchemaTypeFromProxy(param.Schema, !paramRequired, append(fieldPaths, paramName))
		if err != nil {
			return err
		}

		paramLocation, err := rest.ParseParameterLocation(param.In)
		if err != nil {
			return err
		}

		encoding := rest.EncodingObject{
			AllowReserved: param.AllowReserved,
			Explode:       param.Explode,
		}
		if param.Style != "" {
			style, err := rest.ParseParameterEncodingStyle(param.Style)
			if err != nil {
				return err
			}
			encoding.Style = style
		}

		argument := rest.ArgumentInfo{
			ArgumentInfo: schema.ArgumentInfo{
				Type: schemaType.Encode(),
			},
			HTTP: &rest.RequestParameter{
				Name:           paramName,
				In:             paramLocation,
				Schema:         apiSchema,
				EncodingObject: encoding,
			},
		}
		paramDescription := utils.StripHTMLTags(param.Description)
		if paramDescription != "" {
			argument.Description = &paramDescription
		}

		oc.Arguments[paramName] = argument
	}

	return nil
}

func (oc *oas3OperationBuilder) getContentType(contents *orderedmap.Map[string, *v3.MediaType]) (string, *v3.MediaType) {
	var contentType string
	var media *v3.MediaType
	for _, ct := range preferredContentTypes {
		for iter := contents.First(); iter != nil; iter = iter.Next() {
			key := iter.Key()
			value := iter.Value()
			if strings.HasPrefix(key, ct) && value != nil {
				return key, value
			}

			if media == nil && value != nil && (len(oc.builder.AllowedContentTypes) == 0 || slices.Contains(oc.builder.AllowedContentTypes, key)) {
				contentType = key
				media = value
			}
		}
	}

	return contentType, media
}

func (oc *oas3OperationBuilder) convertRequestBody(reqBody *v3.RequestBody, apiPath string, fieldPaths []string) (*rest.RequestBody, schema.TypeEncoder, error) {
	if reqBody == nil || reqBody.Content == nil {
		return nil, nil, nil
	}

	contentType, content := oc.getContentType(reqBody.Content)

	bodyRequired := false
	if reqBody.Required != nil && *reqBody.Required {
		bodyRequired = true
	}
	location := rest.InBody
	if contentType == rest.ContentTypeFormURLEncoded {
		location = rest.InQuery
	}
	schemaType, typeSchema, err := newOAS3SchemaBuilder(oc.builder, apiPath, location, true).
		getSchemaTypeFromProxy(content.Schema, !bodyRequired, fieldPaths)
	if err != nil {
		return nil, nil, err
	}

	if typeSchema == nil {
		return nil, nil, nil
	}

	bodyResult := &rest.RequestBody{
		ContentType: contentType,
	}

	if content.Encoding == nil || content.Encoding.Len() == 0 {
		return bodyResult, schemaType, nil
	}

	bodyResult.Encoding = make(map[string]rest.EncodingObject)
	for iter := content.Encoding.First(); iter != nil; iter = iter.Next() {
		encodingValue := iter.Value()
		if encodingValue == nil {
			continue
		}

		item := rest.EncodingObject{
			ContentType:   utils.SplitStringsAndTrimSpaces(encodingValue.ContentType, ","),
			AllowReserved: encodingValue.AllowReserved,
			Explode:       encodingValue.Explode,
		}

		if encodingValue.Style != "" {
			style, err := rest.ParseParameterEncodingStyle(encodingValue.Style)
			if err != nil {
				return nil, nil, err
			}
			item.Style = style
		}

		if encodingValue.Headers != nil && encodingValue.Headers.Len() > 0 {
			item.Headers = make(map[string]rest.RequestParameter)
			for encodingHeader := encodingValue.Headers.First(); encodingHeader != nil; encodingHeader = encodingHeader.Next() {
				key := strings.TrimSpace(encodingHeader.Key())
				header := encodingHeader.Value()
				if key == "" || header == nil {
					continue
				}

				ndcType, typeSchema, err := newOAS3SchemaBuilder(oc.builder, apiPath, rest.InHeader, true).
					getSchemaTypeFromProxy(header.Schema, header.AllowEmptyValue, append(fieldPaths, key))
				if err != nil {
					return nil, nil, err
				}

				headerEncoding := rest.EncodingObject{
					AllowReserved: header.AllowReserved,
					Explode:       &header.Explode,
				}

				if header.Style != "" {
					style, err := rest.ParseParameterEncodingStyle(header.Style)
					if err != nil {
						return nil, nil, err
					}
					headerEncoding.Style = style
				}

				argumentName := encodeHeaderArgumentName(key)
				headerParam := rest.RequestParameter{
					ArgumentName:   argumentName,
					Schema:         typeSchema,
					EncodingObject: headerEncoding,
				}

				argument := schema.ArgumentInfo{
					Type: ndcType.Encode(),
				}
				headerDesc := utils.StripHTMLTags(header.Description)
				if headerDesc != "" {
					argument.Description = &headerDesc
				}
				item.Headers[key] = headerParam
				oc.Arguments[argumentName] = rest.ArgumentInfo{
					ArgumentInfo: argument,
					HTTP:         &headerParam,
				}
			}
		}
		bodyResult.Encoding[iter.Key()] = item
	}

	return bodyResult, schemaType, nil
}

func (oc *oas3OperationBuilder) convertResponse(responses *v3.Responses, apiPath string, fieldPaths []string) (schema.TypeEncoder, *rest.Response, error) {
	if responses == nil || responses.Codes == nil || responses.Codes.IsZero() {
		return nil, nil, nil
	}

	var resp *v3.Response
	var statusCode int64
	if responses.Codes != nil && !responses.Codes.IsZero() {
		for r := responses.Codes.First(); r != nil; r = r.Next() {
			if r.Key() == "" {
				continue
			}
			code, err := strconv.ParseInt(r.Key(), 10, 32)
			if err != nil {
				continue
			}

			if isUnsupportedResponseCodes(code) {
				return nil, nil, nil
			} else if code >= 200 && code < 300 {
				resp = r.Value()
				statusCode = code

				break
			}
		}
	}

	// return nullable JSON type if the response content is null
	if resp == nil || resp.Content == nil {
		scalarName := rest.ScalarJSON
		if statusCode == http.StatusNoContent {
			scalarName = rest.ScalarBoolean
		}
		oc.builder.schema.AddScalar(string(scalarName), *defaultScalarTypes[scalarName])

		return schema.NewNullableNamedType(string(scalarName)), &rest.Response{
			ContentType: rest.ContentTypeJSON,
		}, nil
	}

	contentType, bodyContent := oc.getContentType(resp.Content)
	if bodyContent == nil {
		if statusCode == http.StatusNoContent {
			scalarName := rest.ScalarBoolean
			oc.builder.schema.AddScalar(string(scalarName), *defaultScalarTypes[scalarName])

			return schema.NewNullableNamedType(string(scalarName)), &rest.Response{
				ContentType: rest.ContentTypeJSON,
			}, nil
		}

		if contentType != "" {
			scalarName := guessScalarResultTypeFromContentType(contentType)
			oc.builder.schema.AddScalar(string(scalarName), *defaultScalarTypes[scalarName])

			return schema.NewNamedType(string(scalarName)), &rest.Response{
				ContentType: contentType,
			}, nil
		}

		return nil, nil, nil
	}

	schemaResponse := &rest.Response{
		ContentType: contentType,
	}
	if bodyContent.Schema == nil {
		return getResultTypeFromContentType(oc.builder.schema, contentType), schemaResponse, nil
	}

	schemaType, _, err := newOAS3SchemaBuilder(oc.builder, apiPath, rest.InBody, false).
		getSchemaTypeFromProxy(bodyContent.Schema, false, fieldPaths)
	if err != nil {
		return nil, nil, err
	}

	switch contentType {
	case rest.ContentTypeNdJSON:
		// Newline Delimited JSON (ndjson) format represents a stream of structured objects
		// so the response would be wrapped with an array
		return schema.NewArrayType(schemaType), schemaResponse, nil
	default:
		return schemaType, schemaResponse, nil
	}
}

func (oc *oas3OperationBuilder) getOperationDescription(operation *v3.Operation) string {
	if operation.Summary != "" {
		return utils.StripHTMLTags(operation.Summary)
	}
	if operation.Description != "" {
		return utils.StripHTMLTags(operation.Description)
	}

	return strings.ToUpper(oc.method) + " " + oc.pathKey
}
