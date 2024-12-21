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
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
)

type oas2SchemaBuilder struct {
	builder   *OAS2Builder
	apiPath   string
	location  rest.ParameterLocation
	writeMode bool
}

func newOAS2SchemaBuilder(builder *OAS2Builder, apiPath string, location rest.ParameterLocation) *oas2SchemaBuilder {
	return &oas2SchemaBuilder{
		builder:  builder,
		apiPath:  apiPath,
		location: location,
	}
}

// get and convert an OpenAPI data type to a NDC type from parameter
func (oc *oas2SchemaBuilder) getSchemaTypeFromParameter(param *v2.Parameter, fieldPaths []string) (schema.TypeEncoder, error) {
	var typeEncoder schema.TypeEncoder
	nullable := param.Required == nil || !*param.Required

	switch param.Type {
	case "object":
		return nil, fmt.Errorf("%s: unsupported object parameter", strings.Join(fieldPaths, "."))
	case "array":
		if param.Items == nil || param.Items.Type == "" {
			if oc.builder.Strict {
				return nil, fmt.Errorf("%s: array item is empty", strings.Join(fieldPaths, "."))
			}

			typeEncoder = schema.NewArrayType(oc.builder.buildScalarJSON())
		} else {
			itemName, isNull := getScalarFromType(oc.builder.schema, []string{param.Items.Type}, param.Format, param.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
			typeEncoder = schema.NewArrayType(schema.NewNamedType(itemName))
			nullable = nullable || isNull
		}
	default:
		if !isPrimitiveScalar([]string{param.Type}) {
			return nil, fmt.Errorf("%s: unsupported schema type %s", strings.Join(fieldPaths, "."), param.Type)
		}

		scalarName, isNull := getScalarFromType(oc.builder.schema, []string{param.Type}, param.Format, param.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
		typeEncoder = schema.NewNamedType(scalarName)
		nullable = nullable || isNull
	}

	if nullable {
		return schema.NewNullableType(typeEncoder), nil
	}

	return typeEncoder, nil
}

// get and convert an OpenAPI data type to a NDC type
func (oc *oas2SchemaBuilder) getSchemaType(typeSchema *base.Schema, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
	if typeSchema == nil {
		return nil, nil, errParameterSchemaEmpty(fieldPaths)
	}

	if len(typeSchema.AllOf) > 0 {
		return oc.buildUnionSchemaType(typeSchema, typeSchema.AllOf, oasAllOf, fieldPaths)
	}

	if len(typeSchema.AnyOf) > 0 {
		return oc.buildUnionSchemaType(typeSchema, typeSchema.AnyOf, oasAnyOf, fieldPaths)
	}

	if len(typeSchema.OneOf) > 0 {
		return oc.buildUnionSchemaType(typeSchema, typeSchema.OneOf, oasOneOf, fieldPaths)
	}

	var result schema.TypeEncoder
	if len(typeSchema.Type) == 0 {
		if oc.builder.Strict {
			return nil, nil, errParameterSchemaEmpty(fieldPaths)
		}
		result = oc.builder.buildScalarJSON()
		if typeSchema.Nullable != nil && *typeSchema.Nullable {
			result = schema.NewNullableType(result)
		}

		return result, createSchemaFromOpenAPISchema(typeSchema), nil
	}

	if len(typeSchema.Type) > 1 || isPrimitiveScalar(typeSchema.Type) {
		scalarName, nullable := getScalarFromType(oc.builder.schema, typeSchema.Type, typeSchema.Format, typeSchema.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
		result = schema.NewNamedType(scalarName)
		if nullable || (typeSchema.Nullable != nil && *typeSchema.Nullable) {
			result = schema.NewNullableType(result)
		}

		return result, createSchemaFromOpenAPISchema(typeSchema), nil
	}

	typeName := typeSchema.Type[0]
	switch typeName {
	case "object":
		return oc.evalObjectType(typeSchema, false, fieldPaths)
	case "array":
		typeResult := createSchemaFromOpenAPISchema(typeSchema)
		nullable := (typeSchema.Nullable != nil && *typeSchema.Nullable)
		if typeSchema.Items == nil || typeSchema.Items.A == nil {
			if oc.builder.ConvertOptions.Strict {
				return nil, nil, fmt.Errorf("%s: array item is empty", strings.Join(fieldPaths, "."))
			}

			result = oc.builder.buildScalarJSON()
			if nullable {
				result = schema.NewNullableType(result)
			}

			return result, typeResult, nil
		}

		itemName := getSchemaRefTypeNameV2(typeSchema.Items.A.GetReference())
		if itemName != "" {
			itemName := utils.ToPascalCase(itemName)
			result = schema.NewArrayType(schema.NewNamedType(itemName))
		} else {
			itemSchemaA := typeSchema.Items.A.Schema()
			if itemSchemaA != nil {
				itemSchema, propType, err := oc.getSchemaType(itemSchemaA, fieldPaths)
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

		if nullable {
			return schema.NewNullableType(result), typeResult, nil
		}

		return result, typeResult, nil
	default:
		return nil, nil, fmt.Errorf("unsupported schema type %s", typeName)
	}
}

func (oc *oas2SchemaBuilder) evalObjectType(baseSchema *base.Schema, forcePropertiesNullable bool, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
	typeResult := createSchemaFromOpenAPISchema(baseSchema)
	refName := utils.StringSliceToPascalCase(fieldPaths)

	if baseSchema.Properties == nil || baseSchema.Properties.IsZero() {
		// treat no-property objects as a JSON scalar
		var scalarType schema.TypeEncoder = oc.builder.buildScalarJSON()
		if baseSchema.Nullable != nil && *baseSchema.Nullable {
			scalarType = schema.NewNullableType(scalarType)
		}

		return scalarType, typeResult, nil
	}

	xmlSchema := typeResult.XML
	if xmlSchema == nil {
		xmlSchema = &rest.XMLSchema{}
	}

	if xmlSchema.Name == "" {
		xmlSchema.Name = fieldPaths[0]
	}
	object := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    xmlSchema,
	}
	if typeResult.Description != "" {
		object.Description = &typeResult.Description
	}

	for prop := baseSchema.Properties.First(); prop != nil; prop = prop.Next() {
		propName := prop.Key()
		oc.builder.Logger.Debug(
			"property",
			slog.String("name", propName),
			slog.Any("field", fieldPaths))

		nullable := forcePropertiesNullable || !slices.Contains(baseSchema.Required, propName)
		propType, propApiSchema, err := oc.getSchemaTypeFromProxy(prop.Value(), nullable, append(fieldPaths, propName))
		if err != nil {
			return nil, nil, err
		}

		if propType == nil {
			continue
		}

		objField := rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: propType.Encode(),
			},
			HTTP: propApiSchema,
		}

		if propApiSchema == nil {
			propApiSchema = &rest.TypeSchema{
				Type: []string{},
			}
		}

		if propApiSchema.Description != "" {
			objField.Description = &propApiSchema.Description
		}

		object.Fields[propName] = objField
	}

	if isXMLLeafObject(object) {
		object.Fields[xmlValueFieldName] = xmlValueField
	}
	oc.builder.schema.ObjectTypes[refName] = object
	var result schema.TypeEncoder = schema.NewNamedType(refName)
	if baseSchema.Nullable != nil && *baseSchema.Nullable {
		result = schema.NewNullableType(result)
	}

	return result, typeResult, nil
}

// get and convert an OpenAPI data type to a NDC type
func (oc *oas2SchemaBuilder) getSchemaTypeFromProxy(schemaProxy *base.SchemaProxy, nullable bool, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
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

	rawRefName := schemaProxy.GetReference()
	if rawRefName == "" {
		ndcType, typeSchema, err = oc.getSchemaType(innerSchema, fieldPaths)
		if err != nil {
			return nil, nil, err
		}
	} else if typeCache, ok := oc.builder.schemaCache[rawRefName]; ok {
		ndcType = typeCache.Schema
		typeSchema = createSchemaFromOpenAPISchema(innerSchema)
		if typeCache.TypeSchema != nil {
			typeSchema.Type = typeCache.TypeSchema.Type
		}
	} else {
		// return early object from ref
		refName := getSchemaRefTypeNameV2(rawRefName)
		schemaName := utils.ToPascalCase(refName)
		oc.builder.schemaCache[rawRefName] = SchemaInfoCache{
			Name:   schemaName,
			Schema: schema.NewNamedType(schemaName),
		}

		_, ok := oc.builder.schema.ObjectTypes[schemaName]
		if !ok {
			ndcType, typeSchema, err = oc.getSchemaType(innerSchema, []string{refName})
			if err != nil {
				return nil, nil, err
			}
			oc.builder.schemaCache[rawRefName] = SchemaInfoCache{
				Name:       schemaName,
				Schema:     ndcType,
				TypeSchema: typeSchema,
			}
		} else {
			ndcType = schema.NewNamedType(schemaName)
			typeSchema = createSchemaFromOpenAPISchema(innerSchema)
		}
	}

	if ndcType == nil {
		return nil, nil, nil
	}

	if nullable {
		if !isNullableType(ndcType) {
			ndcType = schema.NewNullableType(ndcType)
		}
	}

	return ndcType, typeSchema, nil
}

// Support converting allOf and anyOf to object types with merge strategy
func (oc *oas2SchemaBuilder) buildUnionSchemaType(baseSchema *base.Schema, schemaProxies []*base.SchemaProxy, unionType oasUnionType, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
	proxies, mergedType, isNullable := evalSchemaProxiesSlice(schemaProxies, oc.location)
	nullable := isNullable || (baseSchema.Nullable != nil && *baseSchema.Nullable)
	if mergedType != nil {
		typeEncoder, typeSchema, err := oc.getSchemaType(mergedType, fieldPaths)
		if err != nil {
			return nil, nil, err
		}
		if typeSchema != nil && typeSchema.Description == "" && baseSchema.Description != "" {
			typeSchema.Description = utils.StripHTMLTags(baseSchema.Description)
		}

		return typeEncoder, typeSchema, nil
	}

	switch len(proxies) {
	case 0:
		if len(baseSchema.Type) > 1 || isPrimitiveScalar(baseSchema.Type) {
			scalarName, nullable := getScalarFromType(oc.builder.schema, baseSchema.Type, baseSchema.Format, baseSchema.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
			var result schema.TypeEncoder = schema.NewNamedType(scalarName)
			if nullable {
				result = schema.NewNullableType(result)
			}

			return result, createSchemaFromOpenAPISchema(baseSchema), nil
		}

		if len(baseSchema.Type) == 1 && baseSchema.Type[0] == "object" {
			return oc.evalObjectType(baseSchema, true, fieldPaths)
		}

		return schema.NewNamedType(string(rest.ScalarJSON)), createSchemaFromOpenAPISchema(baseSchema), nil
	case 1:
		typeEncoder, typeSchema, err := oc.getSchemaTypeFromProxy(proxies[0], nullable, fieldPaths)
		if err != nil {
			return nil, nil, err
		}
		if typeSchema != nil && typeSchema.Description == "" && baseSchema.Description != "" {
			typeSchema.Description = utils.StripHTMLTags(baseSchema.Description)
		}

		return typeEncoder, typeSchema, nil
	}

	typeSchema := &rest.TypeSchema{
		Type: []string{"object"},
	}

	if baseSchema.Description != "" {
		typeSchema.Description = utils.StripHTMLTags(baseSchema.Description)
	}

	var readObjectItems []rest.ObjectType
	var writeObjectItems []rest.ObjectType

	for i, item := range proxies {
		enc, ty, err := newOAS2SchemaBuilder(oc.builder, oc.apiPath, oc.location).
			getSchemaTypeFromProxy(item, nullable, append(fieldPaths, strconv.Itoa(i)))
		if err != nil {
			return nil, nil, err
		}

		var readObj rest.ObjectType
		name := getNamedType(enc, false, "")
		isObject := name != "" || !isPrimitiveScalar(ty.Type) && !slices.Contains(ty.Type, "array")
		if isObject {
			readObj, isObject = oc.builder.schema.ObjectTypes[name]
			if isObject {
				readObjectItems = append(readObjectItems, readObj)
			}
		}

		if !isObject {
			ty = &rest.TypeSchema{
				Description: ty.Description,
				Type:        []string{},
			}

			return oc.builder.buildScalarJSON(), ty, nil
		}

		writeName := formatWriteObjectName(name)
		writeObj, ok := oc.builder.schema.ObjectTypes[writeName]
		if !ok {
			writeObj = readObj
		}

		writeObjectItems = append(writeObjectItems, writeObj)
	}

	readObject := rest.ObjectType{
		Fields: map[string]rest.ObjectField{},
	}
	writeObject := rest.ObjectType{
		Fields: map[string]rest.ObjectField{},
	}

	if baseSchema.Description != "" {
		readObject.Description = &baseSchema.Description
		writeObject.Description = &baseSchema.Description
	}

	if err := mergeUnionObjects(oc.builder.schema, &readObject, readObjectItems, unionType, fieldPaths); err != nil {
		return nil, nil, err
	}

	if err := mergeUnionObjects(oc.builder.schema, &writeObject, writeObjectItems, unionType, fieldPaths); err != nil {
		return nil, nil, err
	}

	refName := utils.ToPascalCase(strings.Join(fieldPaths, " "))
	writeRefName := formatWriteObjectName(refName)
	if len(readObject.Fields) > 0 {
		oc.builder.schema.ObjectTypes[refName] = readObject
	}
	if len(writeObject.Fields) > 0 {
		oc.builder.schema.ObjectTypes[writeRefName] = writeObject
	}

	if oc.writeMode && len(writeObject.Fields) > 0 {
		refName = writeRefName
	}

	return schema.NewNamedType(refName), typeSchema, nil
}

func (oc *oas2SchemaBuilder) trimPathPrefix(input string) string {
	if oc.builder.ConvertOptions.TrimPrefix == "" {
		return input
	}

	return strings.TrimPrefix(input, oc.builder.ConvertOptions.TrimPrefix)
}
