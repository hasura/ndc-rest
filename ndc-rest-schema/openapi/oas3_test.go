package openapi

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"gotest.tools/v3/assert"
)

func TestOpenAPIv3ToRESTSchema(t *testing.T) {

	testCases := []struct {
		Name     string
		Source   string
		Expected string
		Schema   string
		Options  ConvertOptions
	}{
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/petstore3/source.json -o ./ndc-rest-schema/openapi/testdata/petstore3/expected.json --trim-prefix /v1 --spec openapi3 --env-prefix PET_STORE
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/petstore3/source.json -o ./ndc-rest-schema/openapi/testdata/petstore3/schema.json --pure --trim-prefix /v1 --spec openapi3 --env-prefix PET_STORE
		{
			Name:     "petstore3",
			Source:   "testdata/petstore3/source.json",
			Expected: "testdata/petstore3/expected.json",
			Schema:   "testdata/petstore3/schema.json",
			Options: ConvertOptions{
				TrimPrefix: "/v1",
				EnvPrefix:  "PET_STORE",
			},
		},
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/onesignal/source.json -o ./ndc-rest-schema/openapi/testdata/onesignal/expected.json --spec openapi3
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/onesignal/source.json -o ./ndc-rest-schema/openapi/testdata/onesignal/schema.json --pure --spec openapi3
		{
			Name:     "onesignal",
			Source:   "testdata/onesignal/source.json",
			Expected: "testdata/onesignal/expected.json",
			Schema:   "testdata/onesignal/schema.json",
			Options:  ConvertOptions{},
		},
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/openai/source.json -o ./ndc-rest-schema/openapi/testdata/openai/expected.json --spec openapi3
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/openai/source.json -o ./ndc-rest-schema/openapi/testdata/openai/schema.json --pure --spec openapi3
		{
			Name:     "openai",
			Source:   "testdata/openai/source.json",
			Expected: "testdata/openai/expected.json",
			Schema:   "testdata/openai/schema.json",
			Options:  ConvertOptions{},
		},
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/prefix3/source.json -o ./ndc-rest-schema/openapi/testdata/prefix3/expected_single_word.json --spec openapi3 --prefix hasura
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/prefix3/source.json -o ./ndc-rest-schema/openapi/testdata/prefix3/expected_single_word.schema.json --pure --spec openapi3 --prefix hasura
		{
			Name:     "prefix3_single_word",
			Source:   "testdata/prefix3/source.json",
			Expected: "testdata/prefix3/expected_single_word.json",
			Schema:   "testdata/prefix3/expected_single_word.schema.json",
			Options: ConvertOptions{
				Prefix: "hasura",
			},
		},
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/prefix3/source.json -o ./ndc-rest-schema/openapi/testdata/prefix3/expected_multi_words.json --spec openapi3 --prefix hasura_one_signal
		// go run ./ndc-rest-schema convert -f ./ndc-rest-schema/openapi/testdata/prefix3/source.json -o ./ndc-rest-schema/openapi/testdata/prefix3/expected_multi_words.schema.json --pure --spec openapi3 --prefix hasura_one_signal
		{
			Name:     "prefix3_multi_words",
			Source:   "testdata/prefix3/source.json",
			Expected: "testdata/prefix3/expected_multi_words.json",
			Schema:   "testdata/prefix3/expected_multi_words.schema.json",
			Options: ConvertOptions{
				Prefix: "hasura_one_signal",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			sourceBytes, err := os.ReadFile(tc.Source)
			assert.NilError(t, err)

			expectedBytes, err := os.ReadFile(tc.Expected)
			assert.NilError(t, err)
			var expected schema.NDCRestSchema
			assert.NilError(t, json.Unmarshal(expectedBytes, &expected))

			output, errs := OpenAPIv3ToNDCSchema(sourceBytes, tc.Options)
			if output == nil {
				t.Fatal(errors.Join(errs...))
			}

			assertRESTSchemaEqual(t, &expected, output)
			assertConnectorSchema(t, tc.Schema, output)
		})
	}

	t.Run("failure_empty", func(t *testing.T) {
		_, err := OpenAPIv3ToNDCSchema([]byte(""), ConvertOptions{})
		assert.ErrorContains(t, errors.Join(err...), "there is nothing in the spec, it's empty")
	})
}

func assertRESTSchemaEqual(t *testing.T, expected *schema.NDCRestSchema, output *schema.NDCRestSchema) {
	t.Helper()
	assert.DeepEqual(t, expected.Settings, output.Settings)
	assert.DeepEqual(t, expected.ScalarTypes, output.ScalarTypes)
	objectBs, _ := json.Marshal(output.ObjectTypes)
	var objectTypes map[string]schema.ObjectType
	assert.NilError(t, json.Unmarshal(objectBs, &objectTypes))
	assert.DeepEqual(t, expected.ObjectTypes, objectTypes)
	assert.DeepEqual(t, expected.Procedures, output.Procedures)
	assert.DeepEqual(t, expected.Functions, output.Functions)
}
