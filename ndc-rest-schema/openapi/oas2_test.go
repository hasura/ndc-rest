package openapi

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/hasura/ndc-rest/ndc-rest-schema/schema"
)

func TestOpenAPIv2ToRESTSchema(t *testing.T) {

	testCases := []struct {
		Name     string
		Source   string
		Options  ConvertOptions
		Expected string
	}{
		// go run . convert -f ./openapi/testdata/jsonplaceholder/swagger.json -o ./openapi/testdata/jsonplaceholder/expected.json --spec oas2 --trim-prefix /v1
		{
			Name:     "jsonplaceholder",
			Source:   "testdata/jsonplaceholder/swagger.json",
			Expected: "testdata/jsonplaceholder/expected.json",
			Options: ConvertOptions{
				TrimPrefix: "/v1",
			},
		},
		// go run . convert -f ./openapi/testdata/petstore2/swagger.json -o ./openapi/testdata/petstore2/expected.json --spec oas2
		{
			Name:     "petstore2",
			Source:   "testdata/petstore2/swagger.json",
			Expected: "testdata/petstore2/expected.json",
		},
		// go run . convert -f ./openapi/testdata/prefix2/source.json -o ./openapi/testdata/prefix2/expected_single_word.json --spec oas2 --prefix hasura
		{
			Name:     "prefix2_single_word",
			Source:   "testdata/prefix2/source.json",
			Expected: "testdata/prefix2/expected_single_word.json",
			Options: ConvertOptions{
				Prefix: "hasura",
			},
		},
		// go run . convert -f ./openapi/testdata/prefix2/source.json -o ./openapi/testdata/prefix2/expected_multi_words.json --spec oas2 --prefix hasura_mock_json
		{
			Name:     "prefix2_single_word",
			Source:   "testdata/prefix2/source.json",
			Expected: "testdata/prefix2/expected_multi_words.json",
			Options: ConvertOptions{
				Prefix: "hasura_mock_json",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			sourceBytes, err := os.ReadFile(tc.Source)
			assertNoError(t, err)

			expectedBytes, err := os.ReadFile(tc.Expected)
			assertNoError(t, err)
			var expected schema.NDCRestSchema
			assertNoError(t, json.Unmarshal(expectedBytes, &expected))

			output, errs := OpenAPIv2ToNDCSchema(sourceBytes, tc.Options)
			if output == nil {
				t.Error(errors.Join(errs...))
				t.FailNow()
			}

			assertRESTSchemaEqual(t, &expected, output)
		})
	}

	t.Run("failure_empty", func(t *testing.T) {
		_, err := OpenAPIv2ToNDCSchema([]byte(""), ConvertOptions{})
		assertError(t, errors.Join(err...), "there is nothing in the spec, it's empty")
	})
}
