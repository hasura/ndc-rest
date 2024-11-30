package internal

import (
	"fmt"
	"log/slog"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	sdkUtils "github.com/hasura/ndc-sdk-go/utils"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

// OAS3Builder the NDC schema builder from OpenAPI 3.0 specification
type OAS3Builder struct {
	*ConvertOptions

	schema *rest.NDCHttpSchema
	// stores prebuilt and evaluating information of component schema types.
	// some undefined schema types aren't stored in either object nor scalar,
	// or self-reference types that haven't added into the object_types map yet.
	// This cache temporarily stores them to avoid infinite recursive reference.
	schemaCache map[string]SchemaInfoCache
}

// SchemaInfoCache stores prebuilt information of component schema types.
type SchemaInfoCache struct {
	Name       string
	Schema     schema.TypeEncoder
	TypeSchema *rest.TypeSchema
}

// NewOAS3Builder creates an OAS3Builder instance
func NewOAS3Builder(options ConvertOptions) *OAS3Builder {
	builder := &OAS3Builder{
		schema:         rest.NewNDCHttpSchema(),
		schemaCache:    make(map[string]SchemaInfoCache),
		ConvertOptions: applyConvertOptions(options),
	}

	return builder
}

func (oc *OAS3Builder) BuildDocumentModel(docModel *libopenapi.DocumentModel[v3.Document]) (*rest.NDCHttpSchema, error) {
	if docModel.Model.Info != nil {
		oc.schema.Settings.Version = docModel.Model.Info.Version
	}

	oc.schema.Settings.Servers = oc.convertServers(docModel.Model.Servers)

	if docModel.Model.Components != nil && docModel.Model.Components.Schemas != nil {
		for cSchema := docModel.Model.Components.Schemas.First(); cSchema != nil; cSchema = cSchema.Next() {
			if err := oc.convertComponentSchemas(cSchema); err != nil {
				return nil, err
			}
		}
	}
	for iterPath := docModel.Model.Paths.PathItems.First(); iterPath != nil; iterPath = iterPath.Next() {
		if err := oc.pathToNDCOperations(iterPath); err != nil {
			return nil, err
		}
	}

	if docModel.Model.Components.SecuritySchemes != nil {
		oc.schema.Settings.SecuritySchemes = make(map[string]rest.SecurityScheme)
		for scheme := docModel.Model.Components.SecuritySchemes.First(); scheme != nil; scheme = scheme.Next() {
			err := oc.convertSecuritySchemes(scheme)
			if err != nil {
				return nil, err
			}
		}
	}
	oc.schema.Settings.Security = convertSecurities(docModel.Model.Security)

	// reevaluate write argument types
	oc.schemaCache = make(map[string]SchemaInfoCache)
	oc.transformWriteSchema()

	return NewNDCBuilder(oc.schema, *oc.ConvertOptions).Build()
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
				URL: sdkUtils.NewEnvString(envName, strings.TrimRight(serverURL, "/")),
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
	result := rest.SecurityScheme{}
	switch securityType {
	case rest.APIKeyScheme:
		inLocation, err := rest.ParseAPIKeyLocation(security.In)
		if err != nil {
			return err
		}

		if inLocation == rest.APIKeyInCookie {
			result.SecuritySchemer = rest.NewCookieAuthConfig()
		} else {
			valueEnv := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key}))
			result.SecuritySchemer = rest.NewAPIKeyAuthConfig(security.Name, inLocation, valueEnv)
		}
	case rest.HTTPAuthScheme:
		switch security.Scheme {
		case string(rest.BasicAuthScheme):
			user := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "USERNAME"}))
			password := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "PASSWORD"}))
			result.SecuritySchemer = rest.NewBasicAuthConfig(user, password)
		default:
			valueEnv := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "TOKEN"}))
			result.SecuritySchemer = rest.NewHTTPAuthConfig(security.Scheme, rest.AuthorizationHeader, valueEnv)
		}
	case rest.OAuth2Scheme:
		if security.Flows == nil {
			return fmt.Errorf("flows of security scheme %s is required", key)
		}

		flows := make(map[rest.OAuthFlowType]rest.OAuthFlow)
		if security.Flows.Implicit != nil {
			flows[rest.ImplicitFlow] = oc.convertV3OAuthFLow(key, security.Flows.Implicit)
		}
		if security.Flows.AuthorizationCode != nil {
			flows[rest.AuthorizationCodeFlow] = oc.convertV3OAuthFLow(key, security.Flows.AuthorizationCode)
		}
		if security.Flows.ClientCredentials != nil {
			clientID := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "CLIENT_ID"}))
			clientSecret := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "CLIENT_SECRET"}))
			flow := oc.convertV3OAuthFLow(key, security.Flows.ClientCredentials)
			flow.ClientID = &clientID
			flow.ClientSecret = &clientSecret

			flows[rest.ClientCredentialsFlow] = flow
		}

		if security.Flows.Password != nil {
			flows[rest.PasswordFlow] = oc.convertV3OAuthFLow(key, security.Flows.Password)
		}

		result.SecuritySchemer = rest.NewOAuth2Config(flows)
	case rest.OpenIDConnectScheme:
		result.SecuritySchemer = rest.NewOpenIDConnectConfig(security.OpenIdConnectUrl)
	case rest.MutualTLSScheme:
		result.SecuritySchemer = rest.NewMutualTLSAuthConfig()
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
		funcGet, funcName, err := newOAS3OperationBuilder(oc, pathKey, "get", pathValue.Parameters).BuildFunction(pathValue.Get)
		if err != nil {
			return err
		}
		if funcGet != nil {
			oc.schema.Functions[funcName] = *funcGet
		}
	}

	procPost, procPostName, err := newOAS3OperationBuilder(oc, pathKey, "post", pathValue.Parameters).BuildProcedure(pathValue.Post)
	if err != nil {
		return err
	}
	if procPost != nil {
		oc.schema.Procedures[procPostName] = *procPost
	}

	procPut, procPutName, err := newOAS3OperationBuilder(oc, pathKey, "put", pathValue.Parameters).BuildProcedure(pathValue.Put)
	if err != nil {
		return err
	}
	if procPut != nil {
		oc.schema.Procedures[procPutName] = *procPut
	}

	procPatch, procPutName, err := newOAS3OperationBuilder(oc, pathKey, "patch", pathValue.Parameters).BuildProcedure(pathValue.Patch)
	if err != nil {
		return err
	}
	if procPatch != nil {
		oc.schema.Procedures[procPutName] = *procPatch
	}

	procDelete, procDeleteName, err := newOAS3OperationBuilder(oc, pathKey, "delete", pathValue.Parameters).BuildProcedure(pathValue.Delete)
	if err != nil {
		return err
	}
	if procDelete != nil {
		oc.schema.Procedures[procDeleteName] = *procDelete
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
	typeEncoder, schemaResult, err := newOAS3SchemaBuilder(oc, "", rest.InBody, false).
		getSchemaType(typeSchema, []string{typeKey})
	if err != nil {
		return err
	}

	var typeName string
	if typeEncoder != nil {
		typeName = getNamedType(typeEncoder, true, "")
	}

	if schemaResult.XML == nil {
		schemaResult.XML = &rest.XMLSchema{}
	}
	if schemaResult.XML.Name == "" {
		schemaResult.XML.Name = typeKey
	}

	cacheKey := "#/components/schemas/" + typeKey
	// treat no-property objects as a Arbitrary JSON scalar
	if typeEncoder == nil || typeName == string(rest.ScalarJSON) {
		refName := utils.ToPascalCase(typeKey)
		scalar := schema.NewScalarType()
		scalar.Representation = schema.NewTypeRepresentationJSON().Encode()
		oc.schema.ScalarTypes[refName] = *scalar
		oc.schemaCache[cacheKey] = SchemaInfoCache{
			Name:       refName,
			Schema:     schema.NewNamedType(refName),
			TypeSchema: schemaResult,
		}
	} else {
		oc.schemaCache[cacheKey] = SchemaInfoCache{
			Name:       typeName,
			Schema:     typeEncoder,
			TypeSchema: schemaResult,
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
	oc.schema.AddScalar(scalarName, *defaultScalarTypes[rest.ScalarJSON])

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
		for key, arg := range proc.Arguments {
			ty, name, _ := oc.populateWriteSchemaType(arg.Type)
			if name == "" {
				continue
			}
			arg.Type = ty
			proc.Arguments[key] = arg
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
				TypeSchema: &rest.TypeSchema{
					Type: []string{"object"},
				},
			}
		}

		writeName := formatWriteObjectName(ty.Name)
		if _, ok := oc.schema.ObjectTypes[writeName]; ok {
			return schema.NewNamedType(writeName).Encode(), writeName, true
		}
		if evaluated {
			return schemaType, ty.Name, false
		}
		objectType, ok := oc.schema.ObjectTypes[ty.Name]
		if !ok {
			return schemaType, ty.Name, false
		}
		writeObject := rest.ObjectType{
			Description: objectType.Description,
			XML:         objectType.XML,
			Fields:      make(map[string]rest.ObjectField),
		}
		var hasWriteField bool
		for key, field := range objectType.Fields {
			ut, name, isInput := oc.populateWriteSchemaType(field.Type)
			if name == "" {
				continue
			}
			writeObject.Fields[key] = rest.ObjectField{
				ObjectField: schema.ObjectField{
					Description: field.Description,
					Type:        ut,
				},
				HTTP: field.HTTP,
			}
			if isInput {
				hasWriteField = true
			}
		}
		if hasWriteField {
			oc.schema.ObjectTypes[writeName] = writeObject

			return schema.NewNamedType(writeName).Encode(), writeName, true
		}

		return schemaType, ty.Name, false
	default:
		return schemaType, getNamedType(schemaType.Interface(), true, ""), false
	}
}

func (oc *OAS3Builder) convertV3OAuthFLow(key string, input *v3.OAuthFlow) rest.OAuthFlow {
	result := rest.OAuthFlow{
		AuthorizationURL: input.AuthorizationUrl,
	}

	tokenURL := sdkUtils.NewEnvStringVariable(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "TOKEN_URL"}))
	if input.TokenUrl != "" {
		tokenURL.Value = &input.TokenUrl
	}
	result.TokenURL = &tokenURL

	if input.RefreshUrl != "" {
		refreshURL := sdkUtils.NewEnvString(utils.StringSliceToConstantCase([]string{oc.EnvPrefix, key, "REFRESH_URL"}), input.TokenUrl)
		result.RefreshURL = &refreshURL
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
