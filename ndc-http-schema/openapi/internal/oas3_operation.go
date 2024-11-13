package internal

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
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
	start := time.Now()
	funcName := formatOperationName(itemGet.OperationId)
	if funcName == "" {
		funcName = buildPathMethodName(oc.pathKey, "get", oc.builder.ConvertOptions)
	}
	if oc.builder.Prefix != "" {
		funcName = utils.StringSliceToCamelCase([]string{oc.builder.Prefix, funcName})
	}

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
	function := rest.OperationInfo{
		Request: &rest.Request{
			URL:      oc.pathKey,
			Method:   "get",
			Security: convertSecurities(itemGet.Security),
			Servers:  oc.builder.convertServers(itemGet.Servers),
			Response: *schemaResponse,
		},
		Description: &description,
		Arguments:   oc.Arguments,
		ResultType:  resultType.Encode(),
	}

	return &function, funcName, nil
}

func (oc *oas3OperationBuilder) BuildProcedure(operation *v3.Operation) (*rest.OperationInfo, string, error) {
	if operation == nil {
		return nil, "", nil
	}
	start := time.Now()
	procName := formatOperationName(operation.OperationId)
	if procName == "" {
		procName = buildPathMethodName(oc.pathKey, oc.method, oc.builder.ConvertOptions)
	}

	if oc.builder.Prefix != "" {
		procName = utils.StringSliceToCamelCase([]string{oc.builder.Prefix, procName})
	}

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
	procedure := rest.OperationInfo{
		Request: &rest.Request{
			URL:         oc.pathKey,
			Method:      oc.method,
			Security:    convertSecurities(operation.Security),
			Servers:     oc.builder.convertServers(operation.Servers),
			RequestBody: reqBody,
			Response:    *schemaResponse,
		},
		Description: &description,
		Arguments:   oc.Arguments,
		ResultType:  resultType.Encode(),
	}

	return &procedure, procName, nil
}

func (oc *oas3OperationBuilder) convertParameters(params []*v3.Parameter, apiPath string, fieldPaths []string) error {
	if len(params) == 0 && len(oc.commonParams) == 0 {
		return nil
	}

	for _, param := range append(params, oc.commonParams...) {
		if param == nil {
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
		paramPaths := append(fieldPaths, paramName)
		schemaType, apiSchema, _, err := newOAS3SchemaBuilder(oc.builder, apiPath, rest.ParameterLocation(param.In), true).
			getSchemaTypeFromProxy(param.Schema, !paramRequired, paramPaths)
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
		if param.Description != "" {
			argument.Description = &param.Description
		}

		oc.Arguments[paramName] = argument
	}

	return nil
}

func (oc *oas3OperationBuilder) convertRequestBody(reqBody *v3.RequestBody, apiPath string, fieldPaths []string) (*rest.RequestBody, schema.TypeEncoder, error) {
	if reqBody == nil || reqBody.Content == nil {
		return nil, nil, nil
	}

	contentType := rest.ContentTypeJSON
	content, ok := reqBody.Content.Get(contentType)
	if !ok {
		contentPair := reqBody.Content.First()
		contentType = contentPair.Key()
		content = contentPair.Value()
	}

	bodyRequired := false
	if reqBody.Required != nil && *reqBody.Required {
		bodyRequired = true
	}
	location := rest.InBody
	if contentType == rest.ContentTypeFormURLEncoded {
		location = rest.InQuery
	}
	schemaType, typeSchema, _, err := newOAS3SchemaBuilder(oc.builder, apiPath, location, true).
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

	if content.Encoding != nil && content.Encoding.Len() > 0 {
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

					ndcType, typeSchema, _, err := newOAS3SchemaBuilder(oc.builder, apiPath, rest.InHeader, true).
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
					if header.Description != "" {
						argument.Description = &header.Description
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
	}
	return bodyResult, schemaType, nil
}

func (oc *oas3OperationBuilder) convertResponse(responses *v3.Responses, apiPath string, fieldPaths []string) (schema.TypeEncoder, *rest.Response, error) {
	if responses == nil || responses.Codes == nil || responses.Codes.IsZero() {
		return nil, nil, nil
	}

	var resp *v3.Response
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
				break
			}
		}
	}

	// return nullable boolean type if the response content is null
	if resp == nil || resp.Content == nil {
		scalarName := string(rest.ScalarBoolean)
		return schema.NewNullableNamedType(scalarName), &rest.Response{
			ContentType: rest.ContentTypeJSON,
		}, nil
	}

	contentType := rest.ContentTypeJSON
	bodyContent, present := resp.Content.Get(contentType)
	if !present {
		if len(oc.builder.AllowedContentTypes) == 0 {
			firstContent := resp.Content.First()
			bodyContent = firstContent.Value()
			contentType = firstContent.Key()
			present = true
		} else {
			for _, ct := range oc.builder.AllowedContentTypes {
				bodyContent, present = resp.Content.Get(ct)
				if present {
					contentType = ct
					break
				}
			}
		}
	}

	if !present {
		return nil, nil, nil
	}

	schemaType, _, _, err := newOAS3SchemaBuilder(oc.builder, apiPath, rest.InBody, false).
		getSchemaTypeFromProxy(bodyContent.Schema, false, fieldPaths)
	if err != nil {
		return nil, nil, err
	}

	schemaResponse := &rest.Response{
		ContentType: contentType,
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
