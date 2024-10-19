package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/ndc-rest-schema/utils"
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
func (oc *oas3SchemaBuilder) getSchemaTypeFromProxy(schemaProxy *base.SchemaProxy, nullable bool, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, bool, error) {
	if schemaProxy == nil {
		return nil, nil, false, errParameterSchemaEmpty(fieldPaths)
	}
	innerSchema := schemaProxy.Schema()
	if innerSchema == nil {
		return nil, nil, false, fmt.Errorf("cannot get schema of $.%s from proxy: %s", strings.Join(fieldPaths, "."), schemaProxy.GetReference())
	}

	var ndcType schema.TypeEncoder
	var typeSchema *rest.TypeSchema
	var isRef bool
	var err error

	rawRefName := schemaProxy.GetReference()
	if rawRefName == "" {
		ndcType, typeSchema, isRef, err = oc.getSchemaType(innerSchema, fieldPaths)
		if err != nil {
			return nil, nil, false, err
		}
	} else if typeCache, ok := oc.builder.schemaCache[rawRefName]; ok {
		isRef = true
		ndcType = typeCache.Schema
		typeSchema = typeCache.TypeSchema
	} else {
		// return early object from ref
		refName := getSchemaRefTypeNameV3(rawRefName)
		schemaName := utils.ToPascalCase(refName)
		isRef = true
		oc.builder.schemaCache[rawRefName] = SchemaInfoCache{
			Name:   schemaName,
			Schema: schema.NewNamedType(schemaName),
		}

		_, ok := oc.builder.schema.ObjectTypes[schemaName]
		if !ok {
			ndcType, typeSchema, _, err = oc.getSchemaType(innerSchema, []string{refName})
			if err != nil {
				return nil, nil, false, err
			}
			typeSchema.Description = innerSchema.Description
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
		return nil, nil, false, nil
	}

	if nullable {
		if !isNullableType(ndcType) {
			ndcType = schema.NewNullableType(ndcType)
		}
	}
	return ndcType, typeSchema, isRef, nil
}

// get and convert an OpenAPI data type to a NDC type
func (oc *oas3SchemaBuilder) getSchemaType(typeSchema *base.Schema, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, bool, error) {
	if typeSchema == nil {
		return nil, nil, false, errParameterSchemaEmpty(fieldPaths)
	}

	nullable := typeSchema.Nullable != nil && *typeSchema.Nullable
	if len(typeSchema.AllOf) > 0 {
		enc, ty, isRef, err := oc.buildAllOfAnyOfSchemaType(typeSchema.AllOf, nullable, fieldPaths)
		if err != nil {
			return nil, nil, false, err
		}
		if ty != nil {
			ty.Description = typeSchema.Description
		}
		return enc, ty, isRef, nil
	}

	if len(typeSchema.AnyOf) > 0 {
		enc, ty, isRef, err := oc.buildAllOfAnyOfSchemaType(typeSchema.AnyOf, true, fieldPaths)
		if err != nil {
			return nil, nil, false, err
		}
		if ty != nil {
			ty.Description = typeSchema.Description
		}
		return enc, ty, isRef, nil
	}

	oneOfLength := len(typeSchema.OneOf)
	if oneOfLength == 1 {
		enc, ty, isRef, err := oc.getSchemaTypeFromProxy(typeSchema.OneOf[0], nullable, fieldPaths)
		if err != nil {
			return nil, nil, false, err
		}
		if ty != nil {
			ty.Description = typeSchema.Description
		}
		return enc, ty, isRef, nil
	}

	typeResult := createSchemaFromOpenAPISchema(typeSchema)
	var isRef bool
	if oneOfLength > 0 || (typeSchema.AdditionalProperties != nil && (typeSchema.AdditionalProperties.B || typeSchema.AdditionalProperties.A != nil)) {
		return oc.builder.buildScalarJSON(), typeResult, false, nil
	}

	var result schema.TypeEncoder
	if len(typeSchema.Type) == 0 {
		if oc.builder.Strict {
			return nil, nil, false, errParameterSchemaEmpty(fieldPaths)
		}
		result = oc.builder.buildScalarJSON()
	} else if len(typeSchema.Type) > 1 || isPrimitiveScalar(typeSchema.Type) {
		scalarName := getScalarFromType(oc.builder.schema, typeSchema.Type, typeSchema.Format, typeSchema.Enum, oc.builder.trimPathPrefix(oc.apiPath), fieldPaths)
		result = schema.NewNamedType(scalarName)
	} else {
		typeName := typeSchema.Type[0]
		switch typeName {
		case "object":
			refName := utils.StringSliceToPascalCase(fieldPaths)
			if typeSchema.Properties == nil || typeSchema.Properties.IsZero() {
				if typeSchema.AdditionalProperties != nil && (typeSchema.AdditionalProperties.A == nil || !typeSchema.AdditionalProperties.B) {
					return nil, nil, false, nil
				}
				// treat no-property objects as a JSON scalar
				return oc.builder.buildScalarJSON(), typeResult, false, nil
			}

			object := rest.ObjectType{
				Fields: make(map[string]rest.ObjectField),
			}
			readObject := rest.ObjectType{
				Fields: make(map[string]rest.ObjectField),
			}
			writeObject := rest.ObjectType{
				Fields: make(map[string]rest.ObjectField),
			}
			if typeSchema.Description != "" {
				object.Description = &typeSchema.Description
				readObject.Description = &typeSchema.Description
				writeObject.Description = &typeSchema.Description
			}

			for prop := typeSchema.Properties.First(); prop != nil; prop = prop.Next() {
				propName := prop.Key()
				oc.builder.Logger.Debug(
					"property",
					slog.String("name", propName),
					slog.Any("field", fieldPaths))
				nullable := !slices.Contains(typeSchema.Required, propName)
				propType, propApiSchema, _, err := oc.getSchemaTypeFromProxy(prop.Value(), nullable, append(fieldPaths, propName))
				if err != nil {
					return nil, nil, false, err
				}
				if propType == nil {
					continue
				}
				oc.builder.typeUsageCounter.Add(getNamedType(propType, true, ""), 1)
				objField := rest.ObjectField{
					ObjectField: schema.ObjectField{
						Type: propType.Encode(),
					},
					Rest: propApiSchema,
				}
				if propApiSchema.Description != "" {
					objField.Description = &propApiSchema.Description
				}

				if !propApiSchema.ReadOnly && !propApiSchema.WriteOnly {
					object.Fields[propName] = objField
				} else if !oc.writeMode && propApiSchema.ReadOnly {
					readObject.Fields[propName] = objField
				} else {
					writeObject.Fields[propName] = objField
				}
			}
			if len(readObject.Fields) == 0 && len(writeObject.Fields) == 0 {
				oc.builder.schema.ObjectTypes[refName] = object
				result = schema.NewNamedType(refName)
			} else {
				for key, field := range object.Fields {
					readObject.Fields[key] = field
					writeObject.Fields[key] = field
				}
				writeRefName := formatWriteObjectName(refName)
				oc.builder.schema.ObjectTypes[refName] = readObject
				oc.builder.schema.ObjectTypes[writeRefName] = writeObject
				if oc.writeMode {
					result = schema.NewNamedType(writeRefName)
				} else {
					result = schema.NewNamedType(refName)
				}
			}
		case "array":
			if typeSchema.Items == nil || typeSchema.Items.A == nil {
				return nil, nil, false, errors.New("array item is empty")
			}

			itemName := getSchemaRefTypeNameV3(typeSchema.Items.A.GetReference())
			if itemName != "" {
				result = schema.NewArrayType(schema.NewNamedType(utils.ToPascalCase(itemName)))
			} else {
				itemSchemaA := typeSchema.Items.A.Schema()
				if itemSchemaA != nil {
					itemSchema, propType, _isRef, err := oc.getSchemaType(itemSchemaA, fieldPaths)
					if err != nil {
						return nil, nil, isRef, err
					}
					if itemSchema != nil {
						result = schema.NewArrayType(itemSchema)
					} else {
						result = schema.NewArrayType(oc.builder.buildScalarJSON())
					}

					typeResult.Items = propType
					isRef = _isRef
				}
			}

			if result == nil {
				return nil, nil, false, fmt.Errorf("cannot parse type reference name: %s", typeSchema.Items.A.GetReference())
			}
		default:
			return nil, nil, false, fmt.Errorf("unsupported schema type %s", typeName)
		}
	}

	return result, typeResult, isRef, nil
}

// Support converting allOf and anyOf to object types with merge strategy
func (oc *oas3SchemaBuilder) buildAllOfAnyOfSchemaType(schemaProxies []*base.SchemaProxy, nullable bool, fieldPaths []string) (schema.TypeEncoder, *rest.TypeSchema, bool, error) {
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
		itemFieldPaths := append(fieldPaths, strconv.Itoa(i))
		enc, ty, isRef, err := oc.getSchemaTypeFromProxy(item, nullable, itemFieldPaths)
		if err != nil {
			return nil, nil, false, err
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
			if !isRef {
				delete(oc.builder.schema.ObjectTypes, name)
				delete(oc.builder.schema.ObjectTypes, writeName)
				delete(oc.builder.schema.ScalarTypes, name)
			}
			// TODO: should we keep the original anyOf or allOf type schema
			ty = &rest.TypeSchema{
				Description: ty.Description,
			}
			return oc.builder.buildScalarJSON(), ty, false, nil
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
					oc.builder.typeUsageCounter.Add(getNamedType(v.Type.Interface(), true, ""), 1)
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
					oc.builder.typeUsageCounter.Add(getNamedType(v.Type.Interface(), true, ""), 1)
					writeObject.Fields[k] = v
				}
			}
		}
		if !isRef {
			delete(oc.builder.schema.ObjectTypes, name)
			delete(oc.builder.schema.ObjectTypes, writeName)
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

	return schema.NewNamedType(refName), typeSchema, false, nil
}
