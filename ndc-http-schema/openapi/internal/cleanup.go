package internal

import (
	"errors"
	"fmt"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

type ndcSchemaCleaner struct {
	UsedTypes map[string]bool
	Schema    *rest.NDCHttpSchema
}

func cleanUnusedSchemaTypes(httpSchema *rest.NDCHttpSchema) error {
	cleaner := &ndcSchemaCleaner{
		UsedTypes: make(map[string]bool),
		Schema:    httpSchema,
	}

	if err := cleaner.Validate(); err != nil {
		return err
	}

	cleaner.cleanupUnusedTypes()

	return nil
}

// Validate checks if the schema is valid
func (nsc *ndcSchemaCleaner) Validate() error {
	for key, operation := range nsc.Schema.Functions {
		if err := nsc.validateOperation(key, operation); err != nil {
			return err
		}
	}

	for key, operation := range nsc.Schema.Procedures {
		if err := nsc.validateOperation(key, operation); err != nil {
			return err
		}
	}

	return nil
}

func (nsc *ndcSchemaCleaner) cleanupUnusedTypes() {
	for objectName := range nsc.Schema.ObjectTypes {
		if _, ok := nsc.UsedTypes[objectName]; !ok {
			delete(nsc.Schema.ObjectTypes, objectName)
		}
	}

	for scalarName := range nsc.Schema.ScalarTypes {
		if _, ok := nsc.UsedTypes[scalarName]; !ok {
			delete(nsc.Schema.ScalarTypes, scalarName)
		}
	}
}

// recursively validate and clean unused objects as well as their inner properties
func (nsc *ndcSchemaCleaner) validateOperation(operationName string, operation rest.OperationInfo) error {
	for key, field := range operation.Arguments {
		if err := nsc.validateType(field.Type); err != nil {
			return fmt.Errorf("%s: arguments.%s: %w", operationName, key, err)
		}
	}

	if err := nsc.validateType(operation.ResultType); err != nil {
		return fmt.Errorf("%s: result_type: %w", operationName, err)
	}

	return nil
}

// recursively validate used types as well as their inner properties
func (nsc *ndcSchemaCleaner) validateType(schemaType schema.Type) error {
	typeName := getNamedType(schemaType.Interface(), true, "")
	if typeName == "" {
		return errors.New("named type is empty")
	}

	if _, ok := nsc.UsedTypes[typeName]; ok {
		return nil
	}

	if _, ok := nsc.Schema.ScalarTypes[typeName]; ok {
		nsc.UsedTypes[typeName] = true

		return nil
	}

	objectType, ok := nsc.Schema.ObjectTypes[typeName]
	if !ok {
		return errors.New(typeName + ": named type does not exist")
	}

	nsc.UsedTypes[typeName] = true
	for key, field := range objectType.Fields {
		if err := nsc.validateType(field.Type); err != nil {
			return fmt.Errorf("%s.%s: %w", typeName, key, err)
		}
	}

	return nil
}
