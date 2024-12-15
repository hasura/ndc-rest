package internal

import (
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
)

type oas2OperationBuilder struct {
	builder   *OAS2Builder
	pathKey   string
	method    string
	Arguments map[string]rest.ArgumentInfo
}

func newOAS2OperationBuilder(builder *OAS2Builder, pathKey string, method string) *oas2OperationBuilder {
	return &oas2OperationBuilder{
		builder:   builder,
		pathKey:   pathKey,
		method:    method,
		Arguments: make(map[string]rest.ArgumentInfo),
	}
}

// BuildFunction build a HTTP NDC function information from OpenAPI v2 operation
func (oc *oas2OperationBuilder) BuildFunction(operation *v2.Operation, commonParams []*v2.Parameter) (*rest.OperationInfo, string, error) {
	if operation == nil {
		return nil, "", nil
	}

	funcName := buildUniqueOperationName(oc.builder.schema, operation.OperationId, oc.pathKey, oc.method, oc.builder.ConvertOptions)
	oc.builder.Logger.Info("function",
		slog.String("name", funcName),
		slog.String("path", oc.pathKey),
	)

	resultType, response, err := oc.convertResponse(operation, []string{funcName, "Result"})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", oc.pathKey, err)
	}
	if resultType == nil {
		return nil, "", nil
	}
	reqBody, err := oc.convertParameters(operation, commonParams, []string{funcName})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", funcName, err)
	}

	description := oc.getOperationDescription(operation)
	requestURL, arguments, err := evalOperationPath(oc.builder.schema, oc.pathKey, oc.Arguments)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", funcName, err)
	}
	function := rest.OperationInfo{
		Request: &rest.Request{
			URL:         requestURL,
			Method:      "get",
			RequestBody: reqBody,
			Response:    *response,
			Security:    convertSecurities(operation.Security),
		},
		Description: &description,
		Arguments:   arguments,
		ResultType:  resultType.Encode(),
	}

	return &function, funcName, nil
}

// BuildProcedure build a HTTP NDC function information from OpenAPI v2 operation
func (oc *oas2OperationBuilder) BuildProcedure(operation *v2.Operation, commonParams []*v2.Parameter) (*rest.OperationInfo, string, error) {
	if operation == nil {
		return nil, "", nil
	}

	procName := buildUniqueOperationName(oc.builder.schema, operation.OperationId, oc.pathKey, oc.method, oc.builder.ConvertOptions)

	oc.builder.Logger.Info("procedure",
		slog.String("name", procName),
		slog.String("path", oc.pathKey),
		slog.String("method", oc.method),
	)

	resultType, response, err := oc.convertResponse(operation, []string{procName, "Result"})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", oc.pathKey, err)
	}

	if resultType == nil {
		return nil, "", nil
	}

	reqBody, err := oc.convertParameters(operation, commonParams, []string{procName})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", oc.pathKey, err)
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
			RequestBody: reqBody,
			Security:    convertSecurities(operation.Security),
			Response:    *response,
		},
		Description: &description,
		Arguments:   arguments,
		ResultType:  resultType.Encode(),
	}

	return &procedure, procName, nil
}

func (oc *oas2OperationBuilder) convertParameters(operation *v2.Operation, commonParams []*v2.Parameter, fieldPaths []string) (*rest.RequestBody, error) {
	if operation == nil || (len(operation.Parameters) == 0 && len(commonParams) == 0) {
		return nil, nil
	}

	contentType := oc.getContentTypeV2(operation.Consumes)
	if contentType == "" {
		contentType = rest.ContentTypeJSON
	}

	var requestBody *rest.RequestBody
	formData := rest.TypeSchema{
		Type: []string{"object"},
	}
	formDataObject := rest.ObjectType{
		Fields: map[string]rest.ObjectField{},
	}
	for _, param := range append(operation.Parameters, commonParams...) {
		if param == nil {
			continue
		}
		paramName := param.Name
		if paramName == "" {
			return nil, errParameterNameRequired
		}

		var typeEncoder schema.TypeEncoder
		var typeSchema *rest.TypeSchema
		var err error

		paramRequired := false
		if param.Required != nil && *param.Required {
			paramRequired = true
		}

		switch {
		case param.Type != "":
			typeEncoder, err = newOAS2SchemaBuilder(oc.builder, oc.pathKey, rest.ParameterLocation(param.In)).getSchemaTypeFromParameter(param, fieldPaths)
			if err != nil {
				return nil, err
			}
			typeSchema = &rest.TypeSchema{
				Type:    evaluateOpenAPITypes([]string{param.Type}),
				Pattern: param.Pattern,
			}
			if param.Maximum != nil {
				maximum := float64(*param.Maximum)
				typeSchema.Maximum = &maximum
			}
			if param.Minimum != nil {
				minimum := float64(*param.Minimum)
				typeSchema.Minimum = &minimum
			}
			if param.MaxLength != nil {
				maxLength := int64(*param.MaxLength)
				typeSchema.MaxLength = &maxLength
			}
			if param.MinLength != nil {
				minLength := int64(*param.MinLength)
				typeSchema.MinLength = &minLength
			}
		case param.Schema != nil:
			typeEncoder, typeSchema, err = newOAS2SchemaBuilder(oc.builder, oc.pathKey, rest.ParameterLocation(param.In)).
				getSchemaTypeFromProxy(param.Schema, !paramRequired, fieldPaths)
			if err != nil {
				return nil, err
			}
		default:
			typeEncoder = oc.builder.buildScalarJSON()
			typeSchema = &rest.TypeSchema{
				Type: []string{},
			}
		}

		paramLocation, err := rest.ParseParameterLocation(param.In)
		if err != nil {
			return nil, err
		}

		schemaType := typeEncoder.Encode()
		argument := rest.ArgumentInfo{
			ArgumentInfo: schema.ArgumentInfo{
				Type: schemaType,
			},
		}
		if param.Description != "" {
			description := utils.StripHTMLTags(param.Description)
			if description != "" {
				argument.Description = &description
			}
		}

		switch paramLocation {
		case rest.InBody:
			argument.HTTP = &rest.RequestParameter{
				In:     rest.InBody,
				Schema: typeSchema,
			}
			oc.Arguments[rest.BodyKey] = argument
			requestBody = &rest.RequestBody{
				ContentType: contentType,
			}
		case rest.InFormData:
			if typeSchema != nil {
				param := rest.ObjectField{
					ObjectField: schema.ObjectField{
						Type: argument.Type,
					},
					HTTP: typeSchema,
				}

				if argument.Description != nil {
					desc := utils.StripHTMLTags(*argument.Description)
					if desc != "" {
						param.ObjectField.Description = &desc
					}
				}
				formDataObject.Fields[paramName] = param
			}
		default:
			argument.HTTP = &rest.RequestParameter{
				Name:   paramName,
				In:     paramLocation,
				Schema: typeSchema,
			}
			oc.Arguments[paramName] = argument
		}
	}

	if len(formDataObject.Fields) > 0 {
		bodyName := utils.StringSliceToPascalCase(fieldPaths) + "Body"
		oc.builder.schema.ObjectTypes[bodyName] = formDataObject

		desc := "Form data of " + oc.pathKey
		oc.Arguments["body"] = rest.ArgumentInfo{
			ArgumentInfo: schema.ArgumentInfo{
				Type:        schema.NewNamedType(bodyName).Encode(),
				Description: &desc,
			},
			HTTP: &rest.RequestParameter{
				In:     rest.InFormData,
				Schema: &formData,
			},
		}
		requestBody = &rest.RequestBody{
			ContentType: contentType,
		}
	}

	return requestBody, nil
}

func (oc *oas2OperationBuilder) convertResponse(operation *v2.Operation, fieldPaths []string) (schema.TypeEncoder, *rest.Response, error) {
	if operation.Responses == nil || operation.Responses.Codes == nil || operation.Responses.Codes.IsZero() {
		return nil, nil, nil
	}

	contentType := oc.getContentTypeV2(operation.Produces)
	if contentType == "" {
		oc.builder.Logger.Info("empty content type in response",
			slog.String("path", oc.pathKey),
			slog.String("method", oc.method),
			slog.Any("produces", operation.Produces),
			slog.Any("consumes", operation.Consumes),
		)

		return nil, nil, nil
	}

	var resp *v2.Response
	var statusCode int64
	if operation.Responses.Codes == nil || operation.Responses.Codes.IsZero() {
		// the response is always successful
		resp = operation.Responses.Default
	} else {
		for r := operation.Responses.Codes.First(); r != nil; r = r.Next() {
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

	response := &rest.Response{
		ContentType: contentType,
	}

	// return nullable boolean type if the response content is null
	if resp == nil || resp.Schema == nil {
		scalarName := rest.ScalarJSON
		if statusCode == 204 {
			scalarName = rest.ScalarBoolean
		}
		oc.builder.schema.AddScalar(string(scalarName), *defaultScalarTypes[scalarName])

		return schema.NewNullableNamedType(string(scalarName)), response, nil
	}

	if resp.Schema == nil {
		return getResultTypeFromContentType(oc.builder.schema, contentType), response, nil
	}

	schemaType, _, err := newOAS2SchemaBuilder(oc.builder, oc.pathKey, rest.InBody).
		getSchemaTypeFromProxy(resp.Schema, false, fieldPaths)
	if err != nil {
		return nil, nil, err
	}

	return schemaType, response, nil
}

func (oc *oas2OperationBuilder) getContentTypeV2(contentTypes []string) string {
	for _, contentType := range preferredContentTypes {
		if len(contentTypes) == 0 || slices.Contains(contentTypes, contentType) {
			return contentType
		}
	}

	if len(oc.builder.ConvertOptions.AllowedContentTypes) == 0 {
		return contentTypes[0]
	}

	for _, ct := range oc.builder.ConvertOptions.AllowedContentTypes {
		if slices.Contains(contentTypes, ct) {
			return ct
		}
	}

	return ""
}

func (oc *oas2OperationBuilder) getOperationDescription(operation *v2.Operation) string {
	if operation.Summary != "" {
		return utils.StripHTMLTags(operation.Summary)
	}
	if operation.Description != "" {
		return utils.StripHTMLTags(operation.Description)
	}

	return strings.ToUpper(oc.method) + " " + oc.pathKey
}
