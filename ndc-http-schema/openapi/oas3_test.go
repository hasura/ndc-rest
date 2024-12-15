package openapi

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
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
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/petstore3/source.json -o ./ndc-http-schema/openapi/testdata/petstore3/expected.json --trim-prefix /v1 --spec openapi3 --env-prefix PET_STORE
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/petstore3/source.json -o ./ndc-http-schema/openapi/testdata/petstore3/schema.json --pure --trim-prefix /v1 --spec openapi3 --env-prefix PET_STORE
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
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/onesignal/source.json -o ./ndc-http-schema/openapi/testdata/onesignal/expected.json --spec openapi3 --no-deprecation
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/onesignal/source.json -o ./ndc-http-schema/openapi/testdata/onesignal/schema.json --pure --spec openapi3 --no-deprecation
		{
			Name:     "onesignal",
			Source:   "testdata/onesignal/source.json",
			Expected: "testdata/onesignal/expected.json",
			Schema:   "testdata/onesignal/schema.json",
			Options: ConvertOptions{
				NoDeprecation: true,
			},
		},
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/openai/source.json -o ./ndc-http-schema/openapi/testdata/openai/expected.json --spec openapi3
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/openai/source.json -o ./ndc-http-schema/openapi/testdata/openai/schema.json --pure --spec openapi3
		{
			Name:     "openai",
			Source:   "testdata/openai/source.json",
			Expected: "testdata/openai/expected.json",
			Schema:   "testdata/openai/schema.json",
			Options:  ConvertOptions{},
		},
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/prefix3/source.json -o ./ndc-http-schema/openapi/testdata/prefix3/expected_single_word.json --spec openapi3 --prefix hasura
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/prefix3/source.json -o ./ndc-http-schema/openapi/testdata/prefix3/expected_single_word.schema.json --pure --spec openapi3 --prefix hasura
		{
			Name:     "prefix3_single_word",
			Source:   "testdata/prefix3/source.json",
			Expected: "testdata/prefix3/expected_single_word.json",
			Schema:   "testdata/prefix3/expected_single_word.schema.json",
			Options: ConvertOptions{
				Prefix: "hasura",
			},
		},
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/prefix3/source.json -o ./ndc-http-schema/openapi/testdata/prefix3/expected_multi_words.json --spec openapi3 --prefix hasura_one_signal
		// go run ./ndc-http-schema convert -f ./ndc-http-schema/openapi/testdata/prefix3/source.json -o ./ndc-http-schema/openapi/testdata/prefix3/expected_multi_words.schema.json --pure --spec openapi3 --prefix hasura_one_signal
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
			var expected schema.NDCHttpSchema
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

func assertRESTSchemaEqual(t *testing.T, expected *schema.NDCHttpSchema, output *schema.NDCHttpSchema) {
	t.Helper()
	assertDeepEqual(t, expected.Settings.Headers, output.Settings.Headers)
	assertDeepEqual(t, expected.Settings.Security, output.Settings.Security)
	assertDeepEqual(t, expected.Settings.SecuritySchemes, output.Settings.SecuritySchemes)
	assertDeepEqual(t, expected.Settings.Version, output.Settings.Version)
	for i, server := range expected.Settings.Servers {
		sv := output.Settings.Servers[i]
		assertDeepEqual(t, server.Headers, sv.Headers)
		assertDeepEqual(t, server.ID, sv.ID)
		assertDeepEqual(t, server.Security, sv.Security)
		assertDeepEqual(t, server.SecuritySchemes, sv.SecuritySchemes)
		assertDeepEqual(t, server.URL, sv.URL)
		assertDeepEqual(t, server.TLS, sv.TLS)
	}
	assertDeepEqual(t, expected.ScalarTypes, output.ScalarTypes)
	objectBs, _ := json.Marshal(output.ObjectTypes)
	var objectTypes map[string]schema.ObjectType
	assert.NilError(t, json.Unmarshal(objectBs, &objectTypes))
	assertDeepEqual(t, expected.ObjectTypes, objectTypes)
	assertDeepEqual(t, expected.Procedures, output.Procedures)
	assertDeepEqual(t, expected.Functions, output.Functions)
}

func assertDeepEqual(t *testing.T, expected any, reality any) {
	t.Helper()
	assert.DeepEqual(t, expected, reality)
}
