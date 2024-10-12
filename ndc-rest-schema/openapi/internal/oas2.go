package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/ndc-rest-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
	"github.com/pb33f/libopenapi/orderedmap"
)

// OAS2Builder the NDC schema builder from OpenAPI 2.0 specification
type OAS2Builder struct {
	*ConvertOptions

	schema           *rest.NDCRestSchema
	typeUsageCounter TypeUsageCounter
}

// NewOAS2Builder creates an OAS3Builder instance
func NewOAS2Builder(schema *rest.NDCRestSchema, options ConvertOptions) *OAS2Builder {
	builder := &OAS2Builder{
		schema:           schema,
		typeUsageCounter: TypeUsageCounter{},
		ConvertOptions:   applyConvertOptions(options),
	}

	setDefaultSettings(builder.schema.Settings, builder.ConvertOptions)
	return builder
}

// Schema returns the inner NDC REST schema
func (oc *OAS2Builder) Schema() *rest.NDCRestSchema {
	return oc.schema
}

func (oc *OAS2Builder) BuildDocumentModel(docModel *libopenapi.DocumentModel[v2.Swagger]) error {
	if docModel.Model.Info != nil {
		oc.schema.Settings.Version = docModel.Model.Info.Version
	}

	if docModel.Model.Host != "" {
		scheme := "https"
		for _, s := range docModel.Model.Schemes {
			if strings.HasPrefix(s, "http") {
				scheme = s
				break
			}
		}
		envName := utils.StringSliceToConstantCase([]string{oc.EnvPrefix, "SERVER_URL"})
		serverURL := fmt.Sprintf("%s://%s%s", scheme, docModel.Model.Host, docModel.Model.BasePath)
		oc.schema.Settings.Servers = append(oc.schema.Settings.Servers, rest.ServerConfig{
			URL: *rest.NewEnvStringTemplate(rest.NewEnvTemplateWithDefault(envName, serverURL)),
		})
	}

	for iterPath := docModel.Model.Paths.PathItems.First(); iterPath != nil; iterPath = iterPath.Next() {
		if err := oc.pathToNDCOperations(iterPath); err != nil {
			return err
		}
	}

	if docModel.Model.Definitions != nil {
		for cSchema := docModel.Model.Definitions.Definitions.First(); cSchema != nil; cSchema = cSchema.Next() {
			if err := oc.convertComponentSchemas(cSchema); err != nil {
				return err
			}
		}
	}

	if docModel.Model.SecurityDefinitions != nil && docModel.Model.SecurityDefinitions.Definitions != nil {
		oc.schema.Settings.SecuritySchemes = make(map[string]rest.SecurityScheme)
		for scheme := docModel.Model.SecurityDefinitions.Definitions.First(); scheme != nil; scheme = scheme.Next() {
			err := oc.convertSecuritySchemes(scheme)
			if err != nil {
				return err
			}
		}
	}

	oc.schema.Settings.Security = convertSecurities(docModel.Model.Security)
	cleanUnusedSchemaTypes(oc.schema, &oc.typeUsageCounter)

	return nil
}

func (oc *OAS2Builder) convertSecuritySchemes(scheme orderedmap.Pair[string, *v2.SecurityScheme]) error {
	key := scheme.Key()
	security := scheme.Value()
	if security == nil {
		return nil
	}
	result := rest.SecurityScheme{}
	switch security.Type {
	case "apiKey":
		result.Type = rest.APIKeyScheme
		inLocation, err := rest.ParseAPIKeyLocation(security.In)
		if err != nil {
			return err
		}
		apiConfig := rest.APIKeyAuthConfig{
			In:   inLocation,
			Name: security.Name,
		}
		result.Value = rest.NewEnvStringTemplate(rest.EnvTemplate{
			Name: utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key}),
		})
		result.APIKeyAuthConfig = &apiConfig
	case "basic":
		result.Type = rest.HTTPAuthScheme
		httpConfig := rest.HTTPAuthConfig{
			Scheme: "Basic",
			Header: "Authorization",
		}
		result.Value = rest.NewEnvStringTemplate(rest.EnvTemplate{
			Name: utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "TOKEN"}),
		})
		result.HTTPAuthConfig = &httpConfig
	case "oauth2":
		var flowType rest.OAuthFlowType
		switch security.Flow {
		case "accessCode":
			flowType = rest.AuthorizationCodeFlow
		case "implicit":
			flowType = rest.ImplicitFlow
		case "password":
			flowType = rest.PasswordFlow
		case "application":
			flowType = rest.ClientCredentialsFlow
		}
		flow := rest.OAuthFlow{
			AuthorizationURL: security.AuthorizationUrl,
			TokenURL:         security.TokenUrl,
		}

		if security.Scopes != nil {
			scopes := make(map[string]string)
			for scope := security.Scopes.Values.First(); scope != nil; scope = scope.Next() {
				scopes[scope.Key()] = scope.Value()
			}
			flow.Scopes = scopes
		}
		result.Type = rest.OAuth2Scheme
		result.OAuth2Config = &rest.OAuth2Config{
			Flows: map[rest.OAuthFlowType]rest.OAuthFlow{
				flowType: flow,
			},
		}
	default:
		return fmt.Errorf("invalid security scheme: %s", security.Type)
	}

	oc.schema.Settings.SecuritySchemes[key] = result
	return nil
}

func (oc *OAS2Builder) pathToNDCOperations(pathItem orderedmap.Pair[string, *v2.PathItem]) error {
	pathKey := pathItem.Key()
	pathValue := pathItem.Value()

	funcGet, err := newOAS2OperationBuilder(oc).BuildFunction(pathKey, pathValue.Get)
	if err != nil {
		return err
	}
	if funcGet != nil {
		oc.schema.Functions = append(oc.schema.Functions, funcGet)
	}

	procPost, err := newOAS2OperationBuilder(oc).BuildProcedure(pathKey, "post", pathValue.Post)
	if err != nil {
		return err
	}
	if procPost != nil {
		oc.schema.Procedures = append(oc.schema.Procedures, procPost)
	}

	procPut, err := newOAS2OperationBuilder(oc).BuildProcedure(pathKey, "put", pathValue.Put)
	if err != nil {
		return err
	}
	if procPut != nil {
		oc.schema.Procedures = append(oc.schema.Procedures, procPut)
	}

	procPatch, err := newOAS2OperationBuilder(oc).BuildProcedure(pathKey, "patch", pathValue.Patch)
	if err != nil {
		return err
	}
	if procPatch != nil {
		oc.schema.Procedures = append(oc.schema.Procedures, procPatch)
	}

	procDelete, err := newOAS2OperationBuilder(oc).BuildProcedure(pathKey, "delete", pathValue.Delete)
	if err != nil {
		return err
	}
	if procDelete != nil {
		oc.schema.Procedures = append(oc.schema.Procedures, procDelete)
	}
	return nil
}

// get and convert an OpenAPI data type to a NDC type
func (oc *OAS2Builder) getSchemaTypeFromProxy(schemaProxy *base.SchemaProxy, nullable bool, apiPath string, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
	if schemaProxy == nil {
		return nil, nil, errParameterSchemaEmpty(fieldPaths)
	}

	innerSchema := schemaProxy.Schema()
	if innerSchema == nil {
		return nil, nil, fmt.Errorf("cannot get schema from proxy: %s", schemaProxy.GetReference())
	}
	var ndcType schema.TypeEncoder
	var typeSchema *rest.TypeSchema
	var err error

	refName := getSchemaRefTypeNameV2(schemaProxy.GetReference())
	// return early object from ref
	if refName != "" && len(innerSchema.Type) > 0 && innerSchema.Type[0] == "object" {
		refName = utils.ToPascalCase(refName)
		ndcType = schema.NewNamedType(refName)
		typeSchema = &rest.TypeSchema{Type: refName}
	} else {
		if innerSchema.Title != "" && !strings.Contains(innerSchema.Title, " ") {
			fieldPaths = []string{utils.ToPascalCase(innerSchema.Title)}
		}
		ndcType, typeSchema, err = oc.getSchemaType(innerSchema, apiPath, fieldPaths)
		if err != nil {
			return nil, nil, err
		}
	}
	if nullable {
		typeSchema.Nullable = true
		if !isNullableType(ndcType) {
			ndcType = schema.NewNullableType(ndcType)
		}
	}
	return ndcType, typeSchema, nil
}

// get and convert an OpenAPI data type to a NDC type from parameter
func (oc *OAS2Builder) getSchemaTypeFromParameter(param *v2.Parameter, apiPath string, fieldPaths []string) (schema.TypeEncoder, error) {
	var result schema.TypeEncoder
	if param.Type == "" {
		if oc.Strict {
			return nil, errParameterSchemaEmpty(fieldPaths)
		}
		result = oc.buildScalarJSON()
	} else if isPrimitiveScalar(param.Type) {
		scalarName := getScalarFromType(oc.schema, []string{param.Type}, param.Format, param.Enum, oc.trimPathPrefix(apiPath), fieldPaths)
		result = schema.NewNamedType(scalarName)
	} else {
		switch param.Type {
		case "object":
			return nil, errors.New("unsupported object parameter")
		case "array":
			if param.Items == nil && param.Items.Type == "" {
				if oc.Strict {
					return nil, errors.New("array item is empty")
				}
				result = schema.NewArrayType(oc.buildScalarJSON())
			} else {
				itemName := getScalarFromType(oc.schema, []string{param.Items.Type}, param.Format, param.Enum, oc.trimPathPrefix(apiPath), fieldPaths)
				result = schema.NewArrayType(schema.NewNamedType(itemName))
			}
		default:
			return nil, fmt.Errorf("unsupported schema type %s", param.Type)
		}
	}

	if param.Required == nil || !*param.Required {
		return schema.NewNullableType(result), nil
	}
	return result, nil
}

// get and convert an OpenAPI data type to a NDC type
func (oc *OAS2Builder) getSchemaType(typeSchema *base.Schema, apiPath string, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
	if typeSchema == nil {
		return nil, nil, errParameterSchemaEmpty(fieldPaths)
	}

	var typeResult *rest.TypeSchema
	if len(typeSchema.AnyOf) > 0 || typeSchema.AdditionalProperties != nil || len(typeSchema.Type) > 1 {
		scalarName := string(rest.ScalarJSON)
		if _, ok := oc.schema.ScalarTypes[scalarName]; !ok {
			oc.schema.ScalarTypes[scalarName] = *defaultScalarTypes[rest.ScalarJSON]
		}
		typeResult = createSchemaFromOpenAPISchema(typeSchema, scalarName)
		return schema.NewNamedType(scalarName), typeResult, nil
	}

	var result schema.TypeEncoder
	if len(typeSchema.Type) == 0 {
		if oc.Strict {
			return nil, nil, errParameterSchemaEmpty(fieldPaths)
		}
		result = oc.buildScalarJSON()
		typeResult = createSchemaFromOpenAPISchema(typeSchema, string(rest.ScalarJSON))
	} else {
		typeName := typeSchema.Type[0]
		if isPrimitiveScalar(typeName) {
			scalarName := getScalarFromType(oc.schema, typeSchema.Type, typeSchema.Format, typeSchema.Enum, oc.trimPathPrefix(apiPath), fieldPaths)
			result = schema.NewNamedType(scalarName)
			typeResult = createSchemaFromOpenAPISchema(typeSchema, scalarName)
		} else {
			typeResult = createSchemaFromOpenAPISchema(typeSchema, "")
			typeResult.Type = typeName
			switch typeName {
			case "object":
				refName := utils.StringSliceToPascalCase(fieldPaths)

				if typeSchema.Properties == nil || typeSchema.Properties.IsZero() {
					// treat no-property objects as a JSON scalar
					oc.schema.ScalarTypes[refName] = *defaultScalarTypes[rest.ScalarJSON]
				} else {
					object := schema.ObjectType{
						Fields: make(schema.ObjectTypeFields),
					}
					if typeSchema.Description != "" {
						object.Description = &typeSchema.Description
					}

					typeResult.Properties = make(map[string]rest.TypeSchema)
					for prop := typeSchema.Properties.First(); prop != nil; prop = prop.Next() {
						propName := prop.Key()
						nullable := !slices.Contains(typeSchema.Required, propName)
						propType, propApiSchema, err := oc.getSchemaTypeFromProxy(prop.Value(), nullable, apiPath, append(fieldPaths, propName))
						if err != nil {
							return nil, nil, err
						}
						objField := schema.ObjectField{
							Type: propType.Encode(),
						}
						if propApiSchema.Description != "" {
							objField.Description = &propApiSchema.Description
						}
						propApiSchema.Nullable = nullable
						typeResult.Properties[propName] = *propApiSchema
						object.Fields[propName] = objField

						oc.typeUsageCounter.Add(getNamedType(propType, true, ""), 1)
					}

					oc.schema.ObjectTypes[refName] = object
				}
				result = schema.NewNamedType(refName)
			case "array":
				if typeSchema.Items == nil || typeSchema.Items.A == nil {
					if oc.ConvertOptions.Strict {
						return nil, nil, errors.New("array item is empty")
					}
					result = schema.NewArrayType(oc.buildScalarJSON())
				} else {
					itemName := getSchemaRefTypeNameV2(typeSchema.Items.A.GetReference())
					if itemName != "" {
						itemName := utils.ToPascalCase(itemName)
						result = schema.NewArrayType(schema.NewNamedType(itemName))
					} else {
						itemSchemaA := typeSchema.Items.A.Schema()
						if itemSchemaA != nil {
							itemSchema, propType, err := oc.getSchemaType(itemSchemaA, apiPath, fieldPaths)
							if err != nil {
								return nil, nil, err
							}

							typeResult.Items = propType
							result = schema.NewArrayType(itemSchema)
						}
					}

					if result == nil {
						return nil, nil, fmt.Errorf("cannot parse type reference name: %s", typeSchema.Items.A.GetReference())
					}
				}

			default:
				return nil, nil, fmt.Errorf("unsupported schema type %s", typeName)
			}
		}
	}

	if typeSchema.Nullable != nil && *typeSchema.Nullable {
		return schema.NewNullableType(result), typeResult, nil
	}
	return result, typeResult, nil
}

func (oc *OAS2Builder) convertComponentSchemas(schemaItem orderedmap.Pair[string, *base.SchemaProxy]) error {
	typeKey := schemaItem.Key()
	typeValue := schemaItem.Value()
	typeSchema := typeValue.Schema()

	oc.Logger.Debug("component schema", slog.String("name", typeKey))
	if typeSchema == nil || !slices.Contains(typeSchema.Type, "object") {
		return nil
	}
	_, _, err := oc.getSchemaType(typeSchema, "", []string{typeKey})
	return err
}

func (oc *OAS2Builder) trimPathPrefix(input string) string {
	if oc.ConvertOptions.TrimPrefix == "" {
		return input
	}
	return strings.TrimPrefix(input, oc.ConvertOptions.TrimPrefix)
}

// build a named type for JSON scalar
func (oc *OAS2Builder) buildScalarJSON() *schema.NamedType {
	scalarName := string(rest.ScalarJSON)
	if _, ok := oc.schema.ScalarTypes[scalarName]; !ok {
		oc.schema.ScalarTypes[scalarName] = *defaultScalarTypes[rest.ScalarJSON]
	}
	return schema.NewNamedType(scalarName)
}
