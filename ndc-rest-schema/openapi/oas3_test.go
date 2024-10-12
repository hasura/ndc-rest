package openapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hasura/ndc-rest/ndc-rest-schema/schema"
)

func TestOpenAPIv3ToRESTSchema(t *testing.T) {

	testCases := []struct {
		Name     string
		Source   string
		Expected string
		Options  ConvertOptions
	}{
		// go run . convert  -f ./openapi/testdata/petstore3/source.json -o ./openapi/testdata/petstore3/expected.json --trim-prefix /v1 --spec openapi3 --env-prefix PET_STORE
		{
			Name:     "petstore3",
			Source:   "testdata/petstore3/source.json",
			Expected: "testdata/petstore3/expected.json",
			Options: ConvertOptions{
				TrimPrefix: "/v1",
				EnvPrefix:  "PET_STORE",
			},
		},
		// go run . convert -f ./openapi/testdata/onesignal/source.json -o ./openapi/testdata/onesignal/expected.json --spec openapi3
		{
			Name:     "onesignal",
			Source:   "testdata/onesignal/source.json",
			Expected: "testdata/onesignal/expected.json",
			Options:  ConvertOptions{},
		},
		// go run . convert -f ./openapi/testdata/openai/source.json -o ./openapi/testdata/openai/expected.json --spec openapi3
		{
			Name:     "openai",
			Source:   "testdata/openai/source.json",
			Expected: "testdata/openai/expected.json",
			Options:  ConvertOptions{},
		},
		// go run . convert -f ./openapi/testdata/prefix3/source.json -o ./openapi/testdata/prefix3/expected_single_word.json --spec openapi3 --prefix hasura
		{
			Name:     "prefix3_single_word",
			Source:   "testdata/prefix3/source.json",
			Expected: "testdata/prefix3/expected_single_word.json",
			Options: ConvertOptions{
				Prefix: "hasura",
			},
		},
		// go run . convert -f ./openapi/testdata/prefix3/source.json -o ./openapi/testdata/prefix3/expected_multi_words.json --spec openapi3 --prefix hasura_one_signal
		{
			Name:     "prefix3_multi_words",
			Source:   "testdata/prefix3/source.json",
			Expected: "testdata/prefix3/expected_multi_words.json",
			Options: ConvertOptions{
				Prefix: "hasura_one_signal",
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

			output, errs := OpenAPIv3ToNDCSchema(sourceBytes, tc.Options)
			if output == nil {
				t.Fatal(errors.Join(errs...))
			}

			assertRESTSchemaEqual(t, &expected, output)
		})
	}

	t.Run("failure_empty", func(t *testing.T) {
		_, err := OpenAPIv3ToNDCSchema([]byte(""), ConvertOptions{})
		assertError(t, errors.Join(err...), "there is nothing in the spec, it's empty")
	})
}

func assertNoError(t *testing.T, err error) {
	if err != nil {
		t.Errorf("expected no error, got: %s", err)
		panic(err)
	}
}

func assertError(t *testing.T, err error, message string) {
	if err == nil {
		t.Error("expected error, got nil")
		t.FailNow()
	} else if !strings.Contains(err.Error(), message) {
		t.Errorf("expected error with content: %s, got: %s", err.Error(), message)
		t.FailNow()
	}
}

func assertDeepEqual(t *testing.T, expected any, reality any, msgs ...string) {
	if reflect.DeepEqual(expected, reality) {
		return
	}

	expectedJson, _ := json.Marshal(expected)
	realityJson, _ := json.Marshal(reality)

	var expected1, reality1 any
	assertNoError(t, json.Unmarshal(expectedJson, &expected1))
	assertNoError(t, json.Unmarshal(realityJson, &reality1))

	if !reflect.DeepEqual(expected1, reality1) {
		t.Errorf("%s: not equal.\nexpected: %s\ngot     : %s", strings.Join(msgs, " "), string(expectedJson), string(realityJson))
		t.FailNow()
	}
}

func assertRESTSchemaEqual(t *testing.T, expected *schema.NDCRestSchema, output *schema.NDCRestSchema) {
	assertDeepEqual(t, expected.Settings, output.Settings, "Settings")
	assertDeepEqual(t, len(expected.ScalarTypes), len(output.ScalarTypes), "ScalarTypes")
	for key, item := range expected.ScalarTypes {
		assertDeepEqual(t, item, output.ScalarTypes[key], fmt.Sprintf("ScalarTypes[%s]", key))
	}
	assertDeepEqual(t, len(expected.ObjectTypes), len(output.ObjectTypes), "ObjectTypes")
	for key, item := range expected.ObjectTypes {
		assertDeepEqual(t, item, output.ObjectTypes[key], fmt.Sprintf("ObjectTypes[%s]", key))
	}
	assertDeepEqual(t, len(expected.Procedures), len(output.Procedures), "Procedures")
	for i, item := range expected.Procedures {
		assertDeepEqual(t, item.Arguments, output.Procedures[i].Arguments, fmt.Sprintf("Procedures[%d].Arguments", i))
		assertDeepEqual(t, item.Description, output.Procedures[i].Description, fmt.Sprintf("Procedures[%d].Description", i))
		assertDeepEqual(t, item.Name, output.Procedures[i].Name, fmt.Sprintf("Procedures[%d].Name", i))
		assertDeepEqual(t, item.Request, output.Procedures[i].Request, fmt.Sprintf("Procedures[%d].Request", i))
		assertDeepEqual(t, item.ResultType, output.Procedures[i].ResultType, fmt.Sprintf("Procedures[%d].ResultType", i))
	}
	assertDeepEqual(t, len(expected.Functions), len(output.Functions), "Functions")
	for i, item := range expected.Functions {
		assertDeepEqual(t, item.Arguments, output.Functions[i].Arguments, fmt.Sprintf("Functions[%d].Arguments", i))
		assertDeepEqual(t, item.Description, output.Functions[i].Description, fmt.Sprintf("Functions[%d].Description", i))
		assertDeepEqual(t, item.Name, output.Functions[i].Name, fmt.Sprintf("Functions[%d].Name", i))
		assertDeepEqual(t, item.Request, output.Functions[i].Request, fmt.Sprintf("Functions[%d].Request", i))
		assertDeepEqual(t, item.ResultType, output.Functions[i].ResultType, fmt.Sprintf("Functions[%d].ResultType", i))
	}
}
