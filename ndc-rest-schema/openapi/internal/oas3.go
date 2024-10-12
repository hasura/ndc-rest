package internal

import (
	"fmt"
	"log/slog"
	"strings"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/ndc-rest-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

// OAS3Builder the NDC schema builder from OpenAPI 3.0 specification
type OAS3Builder struct {
	*ConvertOptions

	schema *rest.NDCRestSchema
	// stores prebuilt and evaluating information of component schema types.
	// some undefined schema types aren't stored in either object nor scalar,
	// or self-reference types that haven't added into the object_types map yet.
	// This cache temporarily stores them to avoid infinite recursive reference.
	schemaCache      map[string]SchemaInfoCache
	typeUsageCounter TypeUsageCounter
}

// SchemaInfoCache stores prebuilt information of component schema types.
type SchemaInfoCache struct {
	Name   string
	Schema schema.TypeEncoder
}

// NewOAS3Builder creates an OAS3Builder instance
func NewOAS3Builder(schema *rest.NDCRestSchema, options ConvertOptions) *OAS3Builder {
	builder := &OAS3Builder{
		schema:           schema,
		schemaCache:      make(map[string]SchemaInfoCache),
		typeUsageCounter: TypeUsageCounter{},
		ConvertOptions:   applyConvertOptions(options),
	}

	setDefaultSettings(builder.schema.Settings, builder.ConvertOptions)
	return builder
}

// Schema returns the inner NDC REST schema
func (oc *OAS3Builder) Schema() *rest.NDCRestSchema {
	return oc.schema
}

func (oc *OAS3Builder) BuildDocumentModel(docModel *libopenapi.DocumentModel[v3.Document]) error {
	if docModel.Model.Info != nil {
		oc.schema.Settings.Version = docModel.Model.Info.Version
	}

	oc.schema.Settings.Servers = oc.convertServers(docModel.Model.Servers)

	if docModel.Model.Components != nil && docModel.Model.Components.Schemas != nil {
		for cSchema := docModel.Model.Components.Schemas.First(); cSchema != nil; cSchema = cSchema.Next() {
			if err := oc.convertComponentSchemas(cSchema); err != nil {
				return err
			}
		}
	}
	for iterPath := docModel.Model.Paths.PathItems.First(); iterPath != nil; iterPath = iterPath.Next() {
		if err := oc.pathToNDCOperations(iterPath); err != nil {
			return err
		}
	}

	if docModel.Model.Components.SecuritySchemes != nil {
		oc.schema.Settings.SecuritySchemes = make(map[string]rest.SecurityScheme)
		for scheme := docModel.Model.Components.SecuritySchemes.First(); scheme != nil; scheme = scheme.Next() {
			err := oc.convertSecuritySchemes(scheme)
			if err != nil {
				return err
			}
		}
	}
	oc.schema.Settings.Security = convertSecurities(docModel.Model.Security)

	// reevaluate write argument types
	oc.schemaCache = make(map[string]SchemaInfoCache)
	oc.transformWriteSchema()
	cleanUnusedSchemaTypes(oc.schema, &oc.typeUsageCounter)

	return nil
}

func (oc *OAS3Builder) convertServers(servers []*v3.Server) []rest.ServerConfig {
	var results []rest.ServerConfig

	for i, server := range servers {
		if server.URL != "" {
			var serverID, envName string
			idExtension := server.Extensions.GetOrZero("x-server-id")
			if idExtension != nil {
				serverID = idExtension.Value
			}
			if serverID != "" {
				envName = utils.StringSliceToConstantCase([]string{oc.ConvertOptions.EnvPrefix, serverID, "SERVER_URL"})
			} else {
				envName = utils.StringSliceToConstantCase([]string{oc.ConvertOptions.EnvPrefix, "SERVER_URL"})
				if i > 0 {
					envName = fmt.Sprintf("%s_%d", envName, i+1)
				}
			}

			serverURL := server.URL
			for variable := server.Variables.First(); variable != nil; variable = variable.Next() {
				value := variable.Value()
				if value == nil || value.Default == "" {
					continue
				}
				key := variable.Key()
				serverURL = strings.ReplaceAll(serverURL, fmt.Sprintf("{%s}", key), value.Default)
			}

			conf := rest.ServerConfig{
				ID:  serverID,
				URL: *rest.NewEnvStringTemplate(rest.NewEnvTemplateWithDefault(envName, serverURL)),
			}
			results = append(results, conf)
		}
	}

	return results
}

func (oc *OAS3Builder) convertSecuritySchemes(scheme orderedmap.Pair[string, *v3.SecurityScheme]) error {
	key := scheme.Key()
	security := scheme.Value()
	if security == nil {
		return nil
	}
	securityType, err := rest.ParseSecuritySchemeType(security.Type)
	if err != nil {
		return err
	}
	result := rest.SecurityScheme{
		Type: securityType,
	}
	switch securityType {
	case rest.APIKeyScheme:
		inLocation, err := rest.ParseAPIKeyLocation(security.In)
		if err != nil {
			return err
		}
		apiConfig := rest.APIKeyAuthConfig{
			In:   inLocation,
			Name: security.Name,
		}
		result.Value = rest.NewEnvStringTemplate(rest.NewEnvTemplate(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key})))
		result.APIKeyAuthConfig = &apiConfig
	case rest.HTTPAuthScheme:
		httpConfig := rest.HTTPAuthConfig{
			Scheme: security.Scheme,
			Header: "Authorization",
		}
		result.Value = rest.NewEnvStringTemplate(rest.NewEnvTemplate(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "TOKEN"})))
		result.HTTPAuthConfig = &httpConfig
	case rest.OAuth2Scheme:
		if security.Flows == nil {
			return fmt.Errorf("flows of security scheme %s is required", key)
		}
		oauthConfig := rest.OAuth2Config{
			Flows: make(map[rest.OAuthFlowType]rest.OAuthFlow),
		}
		if security.Flows.Implicit != nil {
			oauthConfig.Flows[rest.ImplicitFlow] = *convertV3OAuthFLow(security.Flows.Implicit)
		}
		if security.Flows.AuthorizationCode != nil {
			oauthConfig.Flows[rest.AuthorizationCodeFlow] = *convertV3OAuthFLow(security.Flows.AuthorizationCode)
		}
		if security.Flows.ClientCredentials != nil {
			oauthConfig.Flows[rest.ClientCredentialsFlow] = *convertV3OAuthFLow(security.Flows.ClientCredentials)
		}
		if security.Flows.Password != nil {
			oauthConfig.Flows[rest.PasswordFlow] = *convertV3OAuthFLow(security.Flows.Password)
		}
		result.OAuth2Config = &oauthConfig
	case rest.OpenIDConnectScheme:
		result.OpenIDConfig = &rest.OpenIDConfig{
			OpenIDConnectURL: security.OpenIdConnectUrl,
		}
	default:
		return fmt.Errorf("invalid security scheme: %s", security.Type)
	}

	oc.schema.Settings.SecuritySchemes[key] = result
	return nil
}

func (oc *OAS3Builder) pathToNDCOperations(pathItem orderedmap.Pair[string, *v3.PathItem]) error {
	pathKey := pathItem.Key()
	pathValue := pathItem.Value()

	if pathValue.Get != nil {
		funcGet, err := newOAS3OperationBuilder(oc, pathKey, "get").BuildFunction(pathValue.Get)
		if err != nil {
			return err
		}
		if funcGet != nil {
			oc.schema.Functions = append(oc.schema.Functions, funcGet)
		}
	}

	procPost, err := newOAS3OperationBuilder(oc, pathKey, "post").BuildProcedure(pathValue.Post)
	if err != nil {
		return err
	}
	if procPost != nil {
		oc.schema.Procedures = append(oc.schema.Procedures, procPost)
	}

	procPut, err := newOAS3OperationBuilder(oc, pathKey, "put").BuildProcedure(pathValue.Put)
	if err != nil {
		return err
	}
	if procPut != nil {
		oc.schema.Procedures = append(oc.schema.Procedures, procPut)
	}

	procPatch, err := newOAS3OperationBuilder(oc, pathKey, "patch").BuildProcedure(pathValue.Patch)
	if err != nil {
		return err
	}
	if procPatch != nil {
		oc.schema.Procedures = append(oc.schema.Procedures, procPatch)
	}

	procDelete, err := newOAS3OperationBuilder(oc, pathKey, "delete").BuildProcedure(pathValue.Delete)
	if err != nil {
		return err
	}
	if procDelete != nil {
		oc.schema.Procedures = append(oc.schema.Procedures, procDelete)
	}
	return nil
}

func (oc *OAS3Builder) convertComponentSchemas(schemaItem orderedmap.Pair[string, *base.SchemaProxy]) error {
	typeValue := schemaItem.Value()
	typeSchema := typeValue.Schema()

	if typeSchema == nil {
		return nil
	}

	typeKey := schemaItem.Key()
	oc.Logger.Debug("component schema", slog.String("name", typeKey))
	if _, ok := oc.schema.ObjectTypes[typeKey]; ok {
		return nil
	}
	typeEncoder, _, _, err := newOAS3SchemaBuilder(oc, "", rest.InBody, false).
		getSchemaType(typeSchema, []string{typeKey})
	if err != nil {
		return err
	}

	// treat no-property objects as a Arbitrary JSON scalar
	if typeEncoder == nil || getNamedType(typeEncoder, true, "") == string(rest.ScalarJSON) {
		refName := utils.ToPascalCase(typeKey)
		scalar := schema.NewScalarType()
		scalar.Representation = schema.NewTypeRepresentationJSON().Encode()
		oc.schema.ScalarTypes[refName] = *scalar
		oc.schemaCache["#/components/schemas/"+typeKey] = SchemaInfoCache{
			Name:   refName,
			Schema: schema.NewNamedType(refName),
		}
	}

	return err
}

func (oc *OAS3Builder) trimPathPrefix(input string) string {
	if oc.ConvertOptions.TrimPrefix == "" {
		return input
	}
	return strings.TrimPrefix(input, oc.ConvertOptions.TrimPrefix)
}

// build a named type for JSON scalar
func (oc *OAS3Builder) buildScalarJSON() *schema.NamedType {
	scalarName := string(rest.ScalarJSON)
	if _, ok := oc.schema.ScalarTypes[scalarName]; !ok {
		oc.schema.ScalarTypes[scalarName] = *defaultScalarTypes[rest.ScalarJSON]
	}
	return schema.NewNamedType(scalarName)
}

// transform and reassign write object types to arguments
func (oc *OAS3Builder) transformWriteSchema() {
	for _, fn := range oc.schema.Functions {
		for key, arg := range fn.Arguments {
			ty, name, _ := oc.populateWriteSchemaType(arg.Type)
			if name != "" {
				arg.Type = ty
				fn.Arguments[key] = arg
			}
		}
	}
	for _, proc := range oc.schema.Procedures {
		var bodyName string
		for key, arg := range proc.Arguments {
			ty, name, _ := oc.populateWriteSchemaType(arg.Type)
			if name == "" {
				continue
			}
			arg.Type = ty
			proc.Arguments[key] = arg
			if key == "body" {
				bodyName = name
			}
		}

		if bodyName != "" && proc.Request.RequestBody != nil && proc.Request.RequestBody.Schema != nil && !isOASType(proc.Request.RequestBody.Schema.Type) {
			proc.Request.RequestBody.Schema.Type = bodyName
		}
	}
}

func (oc *OAS3Builder) populateWriteSchemaType(schemaType schema.Type) (schema.Type, string, bool) {
	switch ty := schemaType.Interface().(type) {
	case *schema.NullableType:
		ut, name, isInput := oc.populateWriteSchemaType(ty.UnderlyingType)
		return schema.NewNullableType(ut.Interface()).Encode(), name, isInput
	case *schema.ArrayType:
		ut, name, isInput := oc.populateWriteSchemaType(ty.ElementType)
		return schema.NewArrayType(ut.Interface()).Encode(), name, isInput
	case *schema.NamedType:
		_, evaluated := oc.schemaCache[ty.Name]
		if !evaluated {
			oc.schemaCache[ty.Name] = SchemaInfoCache{
				Name:   ty.Name,
				Schema: schema.NewNamedType(ty.Name),
			}
		}

		writeName := formatWriteObjectName(ty.Name)
		if _, ok := oc.schema.ObjectTypes[writeName]; ok {
			oc.typeUsageCounter.Add(writeName, 1)
			oc.typeUsageCounter.Add(ty.Name, -1)
			return schema.NewNamedType(writeName).Encode(), writeName, true
		}
		if evaluated {
			return schemaType, ty.Name, false
		}
		objectType, ok := oc.schema.ObjectTypes[ty.Name]
		if !ok {
			return schemaType, ty.Name, false
		}
		writeObject := schema.ObjectType{
			Description: objectType.Description,
			Fields:      make(schema.ObjectTypeFields),
		}
		var hasWriteField bool
		for key, field := range objectType.Fields {
			ut, name, isInput := oc.populateWriteSchemaType(field.Type)
			if name == "" {
				continue
			}
			writeObject.Fields[key] = schema.ObjectField{
				Description: field.Description,
				Type:        ut,
			}
			if isInput {
				hasWriteField = true
			}
		}
		if hasWriteField {
			oc.typeUsageCounter.Add(writeName, 1)
			oc.typeUsageCounter.Add(ty.Name, -1)
			oc.schema.ObjectTypes[writeName] = writeObject
			return schema.NewNamedType(writeName).Encode(), writeName, true
		}
		return schemaType, ty.Name, false
	default:
		return schemaType, getNamedType(schemaType.Interface(), true, ""), false
	}
}

func convertV3OAuthFLow(input *v3.OAuthFlow) *rest.OAuthFlow {
	result := &rest.OAuthFlow{
		AuthorizationURL: input.AuthorizationUrl,
		TokenURL:         input.TokenUrl,
		RefreshURL:       input.RefreshUrl,
	}

	if input.Scopes != nil {
		scopes := make(map[string]string)
		for iter := input.Scopes.First(); iter != nil; iter = iter.Next() {
			key := iter.Key()
			value := iter.Value()
			if key == "" || value == "" {
				continue
			}
			scopes[key] = value
		}
		result.Scopes = scopes
	}

	return result
}
