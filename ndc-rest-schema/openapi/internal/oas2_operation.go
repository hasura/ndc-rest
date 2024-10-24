package internal

import (
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/ndc-rest-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
)

type oas2OperationBuilder struct {
	builder   *OAS2Builder
	Arguments map[string]rest.ArgumentInfo
}

func newOAS2OperationBuilder(builder *OAS2Builder) *oas2OperationBuilder {
	return &oas2OperationBuilder{
		builder:   builder,
		Arguments: make(map[string]rest.ArgumentInfo),
	}
}

// BuildFunction build a REST NDC function information from OpenAPI v2 operation
func (oc *oas2OperationBuilder) BuildFunction(pathKey string, operation *v2.Operation) (*rest.OperationInfo, string, error) {
	if operation == nil {
		return nil, "", nil
	}
	funcName := operation.OperationId
	if funcName == "" {
		funcName = buildPathMethodName(pathKey, "get", oc.builder.ConvertOptions)
	}
	if oc.builder.Prefix != "" {
		funcName = utils.StringSliceToCamelCase([]string{oc.builder.Prefix, funcName})
	}
	oc.builder.Logger.Info("function",
		slog.String("name", funcName),
		slog.String("path", pathKey),
	)

	responseContentType := oc.getResponseContentTypeV2(operation.Produces)
	if responseContentType == "" {
		oc.builder.Logger.Info("supported response content type",
			slog.String("name", funcName),
			slog.String("path", pathKey),
			slog.String("method", "get"),
			slog.Any("produces", operation.Produces),
			slog.Any("consumes", operation.Consumes),
		)
		return nil, "", nil
	}

	resultType, err := oc.convertResponse(operation.Responses, pathKey, []string{funcName, "Result"})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", pathKey, err)
	}
	if resultType == nil {
		return nil, "", nil
	}
	reqBody, err := oc.convertParameters(operation, pathKey, []string{funcName})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", funcName, err)
	}

	description := oc.getOperationDescription(pathKey, "get", operation)
	function := rest.OperationInfo{
		Request: &rest.Request{
			URL:         pathKey,
			Method:      "get",
			RequestBody: reqBody,
			Response: rest.Response{
				ContentType: responseContentType,
			},
			Security: convertSecurities(operation.Security),
		},
		Description: &description,
		Arguments:   oc.Arguments,
		ResultType:  resultType.Encode(),
	}

	return &function, funcName, nil
}

// BuildProcedure build a REST NDC function information from OpenAPI v2 operation
func (oc *oas2OperationBuilder) BuildProcedure(pathKey string, method string, operation *v2.Operation) (*rest.OperationInfo, string, error) {
	if operation == nil {
		return nil, "", nil
	}

	procName := operation.OperationId
	if procName == "" {
		procName = buildPathMethodName(pathKey, method, oc.builder.ConvertOptions)
	}

	if oc.builder.Prefix != "" {
		procName = utils.StringSliceToCamelCase([]string{oc.builder.Prefix, procName})
	}
	oc.builder.Logger.Info("procedure",
		slog.String("name", procName),
		slog.String("path", pathKey),
		slog.String("method", method),
	)

	responseContentType := oc.getResponseContentTypeV2(operation.Produces)
	if responseContentType == "" {
		oc.builder.Logger.Info("supported response content type",
			slog.String("name", procName),
			slog.String("path", pathKey),
			slog.String("method", method),
			slog.Any("produces", operation.Produces),
			slog.Any("consumes", operation.Consumes),
		)
		return nil, "", nil
	}

	resultType, err := oc.convertResponse(operation.Responses, pathKey, []string{procName, "Result"})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", pathKey, err)
	}

	if resultType == nil {
		return nil, "", nil
	}

	reqBody, err := oc.convertParameters(operation, pathKey, []string{procName})
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", pathKey, err)
	}

	description := oc.getOperationDescription(pathKey, method, operation)
	procedure := rest.OperationInfo{
		Request: &rest.Request{
			URL:         pathKey,
			Method:      method,
			RequestBody: reqBody,
			Security:    convertSecurities(operation.Security),
			Response: rest.Response{
				ContentType: responseContentType,
			},
		},
		Description: &description,
		Arguments:   oc.Arguments,
		ResultType:  resultType.Encode(),
	}

	return &procedure, procName, nil
}

func (oc *oas2OperationBuilder) convertParameters(operation *v2.Operation, apiPath string, fieldPaths []string) (*rest.RequestBody, error) {
	if operation == nil || len(operation.Parameters) == 0 {
		return nil, nil
	}

	contentType := rest.ContentTypeJSON
	if len(operation.Consumes) > 0 && !slices.Contains(operation.Consumes, rest.ContentTypeJSON) {
		contentType = operation.Consumes[0]
	}

	var requestBody *rest.RequestBody
	formData := rest.TypeSchema{
		Type: []string{"object"},
	}
	formDataObject := rest.ObjectType{
		Fields: map[string]rest.ObjectField{},
	}
	for _, param := range operation.Parameters {
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

		if param.Type != "" {
			typeEncoder, err = oc.builder.getSchemaTypeFromParameter(param, apiPath, fieldPaths)
			if err != nil {
				return nil, err
			}
			typeSchema = &rest.TypeSchema{
				Type:    []string{param.Type},
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
		} else if param.Schema != nil {
			typeEncoder, typeSchema, err = oc.builder.getSchemaTypeFromProxy(param.Schema, !paramRequired, apiPath, fieldPaths)
			if err != nil {
				return nil, err
			}
		}

		paramLocation, err := rest.ParseParameterLocation(param.In)
		if err != nil {
			return nil, err
		}

		oc.builder.typeUsageCounter.Add(getNamedType(typeEncoder, true, ""), 1)
		schemaType := typeEncoder.Encode()
		argument := rest.ArgumentInfo{
			ArgumentInfo: schema.ArgumentInfo{
				Type: schemaType,
			},
		}
		if param.Description != "" {
			argument.Description = &param.Description
		}

		switch paramLocation {
		case rest.InBody:
			argument.Rest = &rest.RequestParameter{
				In:     rest.InBody,
				Schema: typeSchema,
			}
			oc.Arguments[rest.BodyKey] = argument
			requestBody = &rest.RequestBody{
				ContentType: contentType,
			}
		case rest.InFormData:
			if typeSchema != nil {
				formDataObject.Fields[paramName] = rest.ObjectField{
					ObjectField: schema.ObjectField{
						Type:        argument.Type,
						Description: argument.Description,
					},
					Rest: typeSchema,
				}
			}
		default:
			argument.Rest = &rest.RequestParameter{
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
		oc.builder.typeUsageCounter.Add(bodyName, 1)

		desc := "Form data of " + apiPath
		oc.Arguments["body"] = rest.ArgumentInfo{
			ArgumentInfo: schema.ArgumentInfo{
				Type:        schema.NewNamedType(bodyName).Encode(),
				Description: &desc,
			},
			Rest: &rest.RequestParameter{
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

func (oc *oas2OperationBuilder) convertResponse(responses *v2.Responses, apiPath string, fieldPaths []string) (schema.TypeEncoder, error) {
	if responses == nil || responses.Codes == nil || responses.Codes.IsZero() {
		return nil, nil
	}

	var resp *v2.Response
	if responses.Codes == nil || responses.Codes.IsZero() {
		// the response is always successful
		resp = responses.Default
	} else {
		for r := responses.Codes.First(); r != nil; r = r.Next() {
			if r.Key() == "" {
				continue
			}
			code, err := strconv.ParseInt(r.Key(), 10, 32)
			if err != nil {
				continue
			}

			if isUnsupportedResponseCodes(code) {
				return nil, nil
			} else if code >= 200 && code < 300 {
				resp = r.Value()
				break
			}
		}
	}

	// return nullable boolean type if the response content is null
	if resp == nil || resp.Schema == nil {
		scalarName := string(rest.ScalarBoolean)
		oc.builder.typeUsageCounter.Add(scalarName, 1)
		return schema.NewNullableNamedType(scalarName), nil
	}

	schemaType, _, err := oc.builder.getSchemaTypeFromProxy(resp.Schema, false, apiPath, fieldPaths)
	if err != nil {
		return nil, err
	}
	oc.builder.typeUsageCounter.Add(getNamedType(schemaType, true, ""), 1)
	return schemaType, nil
}

func (oc *oas2OperationBuilder) getResponseContentTypeV2(contentTypes []string) string {
	contentType := rest.ContentTypeJSON
	if len(contentTypes) == 0 || slices.Contains(contentTypes, contentType) {
		return contentType
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

func (oc *oas2OperationBuilder) getOperationDescription(pathKey string, method string, operation *v2.Operation) string {
	if operation.Summary != "" {
		return utils.StripHTMLTags(operation.Summary)
	}
	if operation.Description != "" {
		return utils.StripHTMLTags(operation.Description)
	}
	return strings.ToUpper(method) + " " + pathKey
}
