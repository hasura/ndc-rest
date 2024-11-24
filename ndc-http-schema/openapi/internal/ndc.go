package internal

import (
	"errors"
	"fmt"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
)

// NDCBuilder the NDC schema builder to validate REST connector schema.
type NDCBuilder struct {
	*ConvertOptions

	schema    *rest.NDCHttpSchema
	newSchema *rest.NDCHttpSchema
	usedTypes map[string]string
}

// NewNDCBuilder creates a new NDCBuilder instance.
func NewNDCBuilder(httpSchema *rest.NDCHttpSchema, options ConvertOptions) *NDCBuilder {
	newSchema := rest.NewNDCHttpSchema()
	newSchema.Settings = httpSchema.Settings

	return &NDCBuilder{
		ConvertOptions: &options,
		usedTypes:      make(map[string]string),
		schema:         httpSchema,
		newSchema:      newSchema,
	}
}

// Build validates and build the REST connector schema.
func (ndc *NDCBuilder) Build() (*rest.NDCHttpSchema, error) {
	if err := ndc.validate(); err != nil {
		return nil, err
	}

	return ndc.newSchema, nil
}

// Validate checks if the schema is valid
func (nsc *NDCBuilder) validate() error {
	for key, operation := range nsc.schema.Functions {
		op, err := nsc.validateOperation(key, operation)
		if err != nil {
			return err
		}

		newName := nsc.formatOperationName(key)
		nsc.newSchema.Functions[newName] = *op
	}

	for key, operation := range nsc.schema.Procedures {
		op, err := nsc.validateOperation(key, operation)
		if err != nil {
			return err
		}

		newName := nsc.formatOperationName(key)
		nsc.newSchema.Procedures[newName] = *op
	}

	return nil
}

// recursively validate and clean unused objects as well as their inner properties
func (nsc *NDCBuilder) validateOperation(operationName string, operation rest.OperationInfo) (*rest.OperationInfo, error) {
	result := &rest.OperationInfo{
		Request:     operation.Request,
		Description: operation.Description,
		Arguments:   make(map[string]rest.ArgumentInfo),
	}
	for key, field := range operation.Arguments {
		fieldType, err := nsc.validateType(field.Type)
		if err != nil {
			return nil, fmt.Errorf("%s: arguments.%s: %w", operationName, key, err)
		}
		result.Arguments[key] = rest.ArgumentInfo{
			HTTP: field.HTTP,
			ArgumentInfo: schema.ArgumentInfo{
				Description: field.ArgumentInfo.Description,
				Type:        fieldType.Encode(),
			},
		}
	}

	resultType, err := nsc.validateType(operation.ResultType)
	if err != nil {
		return nil, fmt.Errorf("%s: result_type: %w", operationName, err)
	}
	result.ResultType = resultType.Encode()

	return result, nil
}

// recursively validate used types as well as their inner properties
func (nsc *NDCBuilder) validateType(schemaType schema.Type) (schema.TypeEncoder, error) {
	rawType, err := schemaType.InterfaceT()

	switch t := rawType.(type) {
	case *schema.NullableType:
		underlyingType, err := nsc.validateType(t.UnderlyingType)
		if err != nil {
			return nil, err
		}

		return schema.NewNullableType(underlyingType), nil
	case *schema.ArrayType:
		elementType, err := nsc.validateType(t.ElementType)
		if err != nil {
			return nil, err
		}

		return schema.NewArrayType(elementType), nil
	case *schema.NamedType:
		if t.Name == "" {
			return nil, errors.New("named type is empty")
		}

		if newName, ok := nsc.usedTypes[t.Name]; ok {
			return schema.NewNamedType(newName), nil
		}

		if st, ok := nsc.schema.ScalarTypes[t.Name]; ok {
			newName := t.Name
			if !rest.IsDefaultScalar(t.Name) {
				newName = nsc.formatTypeName(t.Name)
			}
			newNameType := schema.NewNamedType(newName)
			nsc.usedTypes[t.Name] = newName
			if _, ok := nsc.newSchema.ScalarTypes[newName]; !ok {
				nsc.newSchema.ScalarTypes[newName] = st
			}

			return newNameType, nil
		}

		objectType, ok := nsc.schema.ObjectTypes[t.Name]
		if !ok {
			return nil, errors.New(t.Name + ": named type does not exist")
		}

		newName := nsc.formatTypeName(t.Name)
		newNameType := schema.NewNamedType(newName)
		nsc.usedTypes[t.Name] = newName

		newObjectType := rest.ObjectType{
			Description: objectType.Description,
			XML:         objectType.XML,
			Fields:      make(map[string]rest.ObjectField),
		}

		for key, field := range objectType.Fields {
			fieldType, err := nsc.validateType(field.Type)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", t.Name, key, err)
			}
			newObjectType.Fields[key] = rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type:        fieldType.Encode(),
					Description: field.Description,
					Arguments:   field.Arguments,
				},
				HTTP: field.HTTP,
			}
		}
		nsc.newSchema.ObjectTypes[newName] = newObjectType

		return newNameType, nil
	default:
		return nil, err
	}
}

func (nsc *NDCBuilder) formatTypeName(name string) string {
	if nsc.Prefix == "" {
		return name
	}

	return utils.StringSliceToPascalCase([]string{nsc.Prefix, name})
}

func (nsc *NDCBuilder) formatOperationName(name string) string {
	if nsc.Prefix == "" {
		return name
	}

	return utils.StringSliceToCamelCase([]string{nsc.Prefix, name})
}
