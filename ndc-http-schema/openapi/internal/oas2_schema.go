package internal

import (
	"errors"
	"fmt"
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
		return nil, errors.New("unsupported object parameter")
	case "array":
		if param.Items == nil || param.Items.Type == "" {
			if oc.builder.Strict {
				return nil, errors.New("array item is empty")
			}
			typeEncoder = schema.NewArrayType(oc.builder.buildScalarJSON())
		} else {
			itemName, isNull := getScalarFromType(oc.builder.schema, []string{param.Items.Type}, param.Format, param.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
			typeEncoder = schema.NewArrayType(schema.NewNamedType(itemName))
			nullable = nullable || isNull
		}
	default:
		if !isPrimitiveScalar([]string{param.Type}) {
			return nil, fmt.Errorf("unsupported schema type %s", param.Type)
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

	description := utils.StripHTMLTags(typeSchema.Description)
	nullable := typeSchema.Nullable != nil && *typeSchema.Nullable
	if len(typeSchema.AllOf) > 0 {
		enc, ty, err := oc.buildAllOfAnyOfSchemaType(typeSchema.AllOf, nullable, fieldPaths)
		if err != nil {
			return nil, nil, err
		}
		if ty != nil && description != "" {
			ty.Description = description
		}

		return enc, ty, nil
	}

	if len(typeSchema.AnyOf) > 0 {
		enc, ty, err := oc.buildAllOfAnyOfSchemaType(typeSchema.AnyOf, true, fieldPaths)
		if err != nil {
			return nil, nil, err
		}
		if ty != nil && description != "" {
			ty.Description = description
		}

		return enc, ty, nil
	}

	oneOfLength := len(typeSchema.OneOf)
	if oneOfLength == 1 {
		enc, ty, err := oc.getSchemaTypeFromProxy(typeSchema.OneOf[0], nullable, fieldPaths)
		if err != nil {
			return nil, nil, err
		}
		if ty != nil && description != "" {
			ty.Description = description
		}

		return enc, ty, nil
	}

	var typeResult *rest.TypeSchema
	var result schema.TypeEncoder

	if len(typeSchema.Type) == 0 {
		if oc.builder.Strict {
			return nil, nil, errParameterSchemaEmpty(fieldPaths)
		}
		result = oc.builder.buildScalarJSON()
		typeResult = createSchemaFromOpenAPISchema(typeSchema)
	} else {
		typeName := typeSchema.Type[0]
		if isPrimitiveScalar(typeSchema.Type) {
			scalarName, isNull := getScalarFromType(oc.builder.schema, typeSchema.Type, typeSchema.Format, typeSchema.Enum, oc.trimPathPrefix(oc.apiPath), fieldPaths)
			result = schema.NewNamedType(scalarName)
			typeResult = createSchemaFromOpenAPISchema(typeSchema)
			nullable = nullable || isNull
		} else {
			typeResult = createSchemaFromOpenAPISchema(typeSchema)
			switch typeName {
			case "object":
				refName := utils.StringSliceToPascalCase(fieldPaths)

				if typeSchema.Properties == nil || typeSchema.Properties.IsZero() {
					// treat no-property objects as a JSON scalar
					oc.builder.schema.ScalarTypes[refName] = *defaultScalarTypes[rest.ScalarJSON]
				} else {
					object := rest.ObjectType{
						Fields: make(map[string]rest.ObjectField),
					}
					if description != "" {
						object.Description = &description
					}

					for prop := typeSchema.Properties.First(); prop != nil; prop = prop.Next() {
						propName := prop.Key()
						nullable := !slices.Contains(typeSchema.Required, propName)
						propType, propApiSchema, err := oc.getSchemaTypeFromProxy(prop.Value(), nullable, append(fieldPaths, propName))
						if err != nil {
							return nil, nil, err
						}

						objField := rest.ObjectField{
							ObjectField: schema.ObjectField{
								Type: propType.Encode(),
							},
							HTTP: propApiSchema,
						}
						if propApiSchema.Description != "" {
							objField.Description = &propApiSchema.Description
						}
						object.Fields[propName] = objField
					}

					oc.builder.schema.ObjectTypes[refName] = object
				}
				result = schema.NewNamedType(refName)
			case "array":
				if typeSchema.Items == nil || typeSchema.Items.A == nil {
					if oc.builder.ConvertOptions.Strict {
						return nil, nil, errors.New("array item is empty")
					}
					result = schema.NewArrayType(oc.builder.buildScalarJSON())
				} else {
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
				}

			default:
				return nil, nil, fmt.Errorf("unsupported schema type %s", typeName)
			}
		}
	}

	if nullable {
		return schema.NewNullableType(result), typeResult, nil
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
func (oc *oas2SchemaBuilder) buildAllOfAnyOfSchemaType(schemaProxies []*base.SchemaProxy, nullable bool, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, error) {
	proxies, mergedType, isNullable := evalSchemaProxiesSlice(schemaProxies, oc.location)
	nullable = nullable || isNullable

	if mergedType != nil {
		return oc.getSchemaType(mergedType, fieldPaths)
	}

	if len(proxies) == 1 {
		return oc.getSchemaTypeFromProxy(proxies[0], nullable, fieldPaths)
	}
	readObject := rest.ObjectType{
		Fields: map[string]rest.ObjectField{},
	}
	writeObject := rest.ObjectType{
		Fields: map[string]rest.ObjectField{},
	}
	typeSchema := &rest.TypeSchema{
		Type: []string{"object"},
	}

	for i, item := range proxies {
		enc, ty, err := oc.getSchemaTypeFromProxy(item, nullable, append(fieldPaths, strconv.Itoa(i)))
		if err != nil {
			return nil, nil, err
		}

		name := getNamedType(enc, true, "")
		writeName := formatWriteObjectName(name)
		isObject := !isPrimitiveScalar(ty.Type) && !slices.Contains(ty.Type, "array")
		if isObject {
			if _, ok := oc.builder.schema.ScalarTypes[name]; ok {
				isObject = false
			}
		}
		if !isObject {
			// TODO: should we keep the original anyOf or allOf type schema
			ty = &rest.TypeSchema{
				Description: ty.Description,
				Type:        []string{},
			}

			return oc.builder.buildScalarJSON(), ty, nil
		}

		readObj, ok := oc.builder.schema.ObjectTypes[name]
		if ok {
			if readObject.Description == nil && readObj.Description != nil {
				readObject.Description = readObj.Description
				if ty.Description == "" {
					ty.Description = *readObj.Description
				}
			}
			for k, v := range readObj.Fields {
				if _, ok := readObject.Fields[k]; !ok {
					readObject.Fields[k] = v
				}
			}
		}
		writeObj, ok := oc.builder.schema.ObjectTypes[writeName]
		if ok {
			if writeObject.Description == nil && writeObj.Description != nil {
				writeObject.Description = writeObj.Description
			}
			for k, v := range writeObj.Fields {
				if _, ok := writeObject.Fields[k]; !ok {
					writeObject.Fields[k] = v
				}
			}
		}
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
