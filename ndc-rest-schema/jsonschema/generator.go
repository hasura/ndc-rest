package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hasura/ndc-rest/ndc-rest-schema/command"
	"github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/invopop/jsonschema"
)

func main() {
	if err := jsonSchemaConvertConfig(); err != nil {
		panic(fmt.Errorf("failed to write jsonschema for ConvertConfig: %w", err))
	}
	if err := jsonSchemaNdcRESTSchema(); err != nil {
		panic(fmt.Errorf("failed to write jsonschema for NDCRestSchema: %w", err))
	}
}

func jsonSchemaConvertConfig() error {
	r := new(jsonschema.Reflector)
	if err := r.AddGoComments("github.com/hasura/ndc-rest/ndc-rest-schema/command", "../command"); err != nil {
		return err
	}
	reflectSchema := r.Reflect(&command.ConvertConfig{})

	schemaBytes, err := json.MarshalIndent(reflectSchema, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile("convert-config.schema.json", schemaBytes, 0644)
}

func jsonSchemaNdcRESTSchema() error {
	r := new(jsonschema.Reflector)
	if err := r.AddGoComments("github.com/hasura/ndc-rest/ndc-rest-schema/schema", "../schema"); err != nil {
		return err
	}

	reflectSchema := r.Reflect(&schema.NDCRestSchema{})
	schemaBytes, err := json.MarshalIndent(reflectSchema, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile("ndc-rest-schema.schema.json", schemaBytes, 0644)
}
