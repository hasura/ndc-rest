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
)

type oas3SchemaBuilder struct {
	builder   *OAS3Builder
	apiPath   string
	location  rest.ParameterLocation
	writeMode bool
}

func newOAS3SchemaBuilder(builder *OAS3Builder, apiPath string, location rest.ParameterLocation, writeMode bool) *oas3SchemaBuilder {
	return &oas3SchemaBuilder{
		builder:   builder,
		apiPath:   apiPath,
		writeMode: writeMode,
		location:  location,
	}
}

// get and convert an OpenAPI data type to a NDC type
func (oc *oas3SchemaBuilder) getSchemaTypeFromProxy(schemaProxy *base.SchemaProxy, nullable bool, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
	if schemaProxy == nil {
		return nil, nil, errParameterSchemaEmpty(fieldPaths)
	}

	innerSchema := schemaProxy.Schema()
	if innerSchema == nil {
		return nil, nil, fmt.Errorf("cannot get schema of $.%s from proxy: %s", strings.Join(fieldPaths, "."), schemaProxy.GetReference())
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
		refName := getSchemaRefTypeNameV3(rawRefName)
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

// get and convert an OpenAPI data type to a NDC type
func (oc *oas3SchemaBuilder) getSchemaType(typeSchema *base.Schema, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
	if typeSchema == nil {
		return nil, nil, errParameterSchemaEmpty(fieldPaths)
	}

	if oc.builder.ConvertOptions.NoDeprecation && typeSchema.Deprecated != nil && *typeSchema.Deprecated {
		return nil, nil, nil
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

	if typeSchema.AdditionalProperties != nil && (typeSchema.AdditionalProperties.B || typeSchema.AdditionalProperties.A != nil) {
		return oc.builder.buildScalarJSON(), createSchemaFromOpenAPISchema(typeSchema), nil
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
		scalarName, nullable := getScalarFromType(oc.builder.schema, typeSchema.Type, typeSchema.Format, typeSchema.Enum, oc.builder.trimPathPrefix(oc.apiPath), fieldPaths)
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
			if oc.builder.Strict {
				return nil, nil, fmt.Errorf("%s: array item is empty", strings.Join(fieldPaths, "."))
			}

			var result schema.TypeEncoder = oc.builder.buildScalarJSON()
			if nullable {
				result = schema.NewNullableType(result)
			}

			return result, typeResult, nil
		}

		itemName := getSchemaRefTypeNameV3(typeSchema.Items.A.GetReference())
		if itemName != "" {
			result = schema.NewArrayType(schema.NewNamedType(utils.ToPascalCase(itemName)))
		} else {
			itemSchemaA := typeSchema.Items.A.Schema()
			if itemSchemaA != nil {
				itemSchema, propType, err := oc.getSchemaType(itemSchemaA, fieldPaths)
				if err != nil {
					return nil, nil, err
				}
				if itemSchema != nil {
					result = schema.NewArrayType(itemSchema)
				} else {
					result = schema.NewArrayType(oc.builder.buildScalarJSON())
				}

				typeResult.Items = propType
			}
		}

		if result == nil {
			return nil, nil, fmt.Errorf("cannot parse type reference name: %s", typeSchema.Items.A.GetReference())
		}

		if nullable {
			result = schema.NewNullableType(result)
		}

		return result, typeResult, nil
	default:
		return nil, nil, fmt.Errorf("unsupported schema type %s", typeName)
	}
}

func (oc *oas3SchemaBuilder) evalObjectType(baseSchema *base.Schema, forcePropertiesNullable bool, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
	typeResult := createSchemaFromOpenAPISchema(baseSchema)
	refName := utils.StringSliceToPascalCase(fieldPaths)
	if baseSchema.Properties == nil || baseSchema.Properties.IsZero() {
		if baseSchema.AdditionalProperties != nil && (baseSchema.AdditionalProperties.A == nil || !baseSchema.AdditionalProperties.B) {
			return nil, nil, nil
		}
		// treat no-property objects as a JSON scalar
		var scalarType schema.TypeEncoder = oc.builder.buildScalarJSON()
		if baseSchema.Nullable != nil && *baseSchema.Nullable {
			scalarType = schema.NewNullableType(scalarType)
		}

		return scalarType, typeResult, nil
	}

	var result schema.TypeEncoder
	object := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    typeResult.XML,
	}
	readObject := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    typeResult.XML,
	}
	writeObject := rest.ObjectType{
		Fields: make(map[string]rest.ObjectField),
		XML:    typeResult.XML,
	}

	if typeResult.Description != "" {
		object.Description = &typeResult.Description
		readObject.Description = &typeResult.Description
		writeObject.Description = &typeResult.Description
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

		switch {
		case !propApiSchema.ReadOnly && !propApiSchema.WriteOnly:
			object.Fields[propName] = objField
		case !oc.writeMode && propApiSchema.ReadOnly:
			readObject.Fields[propName] = objField
		default:
			writeObject.Fields[propName] = objField
		}
	}

	writeRefName := formatWriteObjectName(refName)
	if len(readObject.Fields) == 0 && len(writeObject.Fields) == 0 {
		if len(object.Fields) > 0 && isXMLLeafObject(object) {
			object.Fields[xmlValueFieldName] = xmlValueField
		}

		oc.builder.schema.ObjectTypes[refName] = object
		result = schema.NewNamedType(refName)
	} else {
		for key, field := range object.Fields {
			readObject.Fields[key] = field
			writeObject.Fields[key] = field
		}

		if len(readObject.Fields) > 0 && isXMLLeafObject(readObject) {
			readObject.Fields[xmlValueFieldName] = xmlValueField
		}

		if len(writeObject.Fields) > 0 && isXMLLeafObject(writeObject) {
			writeObject.Fields[xmlValueFieldName] = xmlValueField
		}

		oc.builder.schema.ObjectTypes[refName] = readObject
		oc.builder.schema.ObjectTypes[writeRefName] = writeObject
		if oc.writeMode {
			result = schema.NewNamedType(writeRefName)
		} else {
			result = schema.NewNamedType(refName)
		}
	}

	if baseSchema.Nullable != nil && *baseSchema.Nullable {
		result = schema.NewNullableType(result)
	}

	return result, typeResult, nil
}

// Support converting oneOf, allOf or anyOf to object types with merge strategy
func (oc *oas3SchemaBuilder) buildUnionSchemaType(baseSchema *base.Schema, schemaProxies []*base.SchemaProxy, unionType oasUnionType, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
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
			scalarName, nullable := getScalarFromType(oc.builder.schema, baseSchema.Type, baseSchema.Format, baseSchema.Enum, oc.builder.trimPathPrefix(oc.apiPath), fieldPaths)
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
		enc, ty, err := newOAS3SchemaBuilder(oc.builder, oc.apiPath, oc.location, false).
			getSchemaTypeFromProxy(item, nullable, append(fieldPaths, strconv.Itoa(i)))
		if err != nil {
			return nil, nil, err
		}

		var readObj rest.ObjectType
		name := getNamedType(enc, false, "")
		isObject := name != "" && !isPrimitiveScalar(ty.Type) && !slices.Contains(ty.Type, "array")
		if isObject {
			readObj, isObject = oc.builder.schema.ObjectTypes[name]
			if isObject {
				readObjectItems = append(readObjectItems, readObj)
			}
		}

		if !isObject {
			ty = &rest.TypeSchema{
				Description: typeSchema.Description,
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

type unionSiblingField struct {
	Type        schema.TypeEncoder
	EnumOneOf   []string
	Description *string
	HTTP        *rest.TypeSchema
}

func mergeUnionObjects(httpSchema *rest.NDCHttpSchema, dest *rest.ObjectType, srcObjects []rest.ObjectType, unionType oasUnionType, fieldPaths []string) error {
	// Find common fields in all objects to merge the type.
	// If they have the same type, we don't need to wrap it with the nullable type.
	objectItemLength := len(srcObjects)
	siblingFields := make(map[string]unionSiblingField)
	for i, object := range srcObjects {
		if i >= objectItemLength-1 {
			break
		}

		for key, field := range object.Fields {
			siblingField, siblingFieldExist := siblingFields[key]
			nextField, ok := srcObjects[i+1].Fields[key]

			if !ok {
				if siblingFieldExist {
					siblingFields[key] = unionSiblingField{
						Type: schema.NewNullableType(schema.NewNamedType(string(rest.ScalarJSON))),
					}
				}

				continue
			}

			newField, ok := mergeUnionTypes(httpSchema, field.Type, nextField.Type, append(fieldPaths, key))
			switch {
			case ok:
				usField := unionSiblingField{
					Type:      newField.Type,
					EnumOneOf: append(siblingField.EnumOneOf, newField.EnumOneOf...),
				}

				switch {
				case siblingFieldExist && siblingField.Description != nil:
					usField.Description = siblingField.Description
				case field.Description != nil:
					usField.Description = field.Description
				case nextField.Description != nil:
					usField.Description = nextField.Description
				}

				switch {
				case len(newField.EnumOneOf) > 0:
					usField.HTTP = &rest.TypeSchema{
						Type: []string{"string"},
					}
				case siblingFieldExist && siblingField.HTTP != nil:
					usField.HTTP = siblingField.HTTP
				case field.HTTP != nil:
					usField.HTTP = field.HTTP
				case nextField.HTTP != nil:
					usField.HTTP = nextField.HTTP
				}

				siblingFields[key] = usField
			case siblingFieldExist:
				newField, _ = mergeUnionTypes(httpSchema, siblingField.Type.Encode(), nextField.Type, append(fieldPaths, key))
				siblingFields[key] = unionSiblingField{
					Type: newField.Type,
				}
			default:
				siblingFields[key] = *newField
			}
		}
	}

	for key, field := range siblingFields {
		fieldType := field.Type
		if len(field.EnumOneOf) > 0 {
			newScalar := schema.NewScalarType()
			newScalar.Representation = schema.NewTypeRepresentationEnum(utils.SliceUnique(field.EnumOneOf)).Encode()

			newName := utils.StringSliceToPascalCase(append(fieldPaths, key, "Enum"))
			httpSchema.ScalarTypes[newName] = *newScalar

			var err error
			fieldType, err = replaceNamedType(field.Type.Encode(), newName)
			if err != nil {
				return fmt.Errorf("%s: failed to replace named type, %w", strings.Join(append(fieldPaths, key), "."), err)
			}
		}

		dest.Fields[key] = rest.ObjectField{
			ObjectField: schema.ObjectField{
				Description: field.Description,
				Type:        fieldType.Encode(),
			},
			HTTP: field.HTTP,
		}
	}

	for _, objectItem := range srcObjects {
		for key, field := range objectItem.Fields {
			if _, ok := siblingFields[key]; ok {
				continue
			}

			// In anyOf and oneOf union objects, the API only requires one of union objects, other types are optional.
			// Because the NDC spec hasn't supported union types yet we make all properties optional to enable autocompletion.
			iType := field.Type.Interface()
			if unionType != oasAllOf && !isNullableType(iType) {
				field.ObjectField.Type = schema.NewNullableType(iType).Encode()
			}

			dest.Fields[key] = field
		}
	}

	return nil
}
