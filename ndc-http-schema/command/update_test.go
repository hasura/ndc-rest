package command

import (
	"encoding/json"
	"log/slog"
	"os"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gotest.tools/v3/assert"
)

func TestUpdateCommand(t *testing.T) {
	testCases := []struct {
		Argument UpdateCommandArguments
		Expected string
	}{
		// go run ./ndc-http-schema update -d ./ndc-http-schema/command/testdata/patch
		{
			Argument: UpdateCommandArguments{
				Dir: "testdata/patch",
			},
			Expected: "testdata/patch/expected.json",
		},
		// go run ./ndc-http-schema update -d ./ndc-http-schema/command/testdata/auth
		{
			Argument: UpdateCommandArguments{
				Dir: "testdata/auth",
			},
			Expected: "testdata/auth/expected.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Argument.Dir, func(t *testing.T) {
			assert.NilError(t, UpdateConfiguration(&tc.Argument, slog.Default()))

			output := readRuntimeSchemaFile(t, tc.Argument.Dir+"/schema.output.json")
			expected := readRuntimeSchemaFile(t, tc.Argument.Dir+"/expected.json")
			assertSchemaEqual(t, expected, output)
		})
	}
}

func readRuntimeSchemaFile(t *testing.T, filePath string) []configuration.NDCHttpRuntimeSchema {
	t.Helper()
	rawBytes, err := os.ReadFile(filePath)
	assert.NilError(t, err)

	var result []configuration.NDCHttpRuntimeSchema
	assert.NilError(t, json.Unmarshal(rawBytes, &result))

	return result
}

func assertSchemaEqual(t *testing.T, expectedSchemas []configuration.NDCHttpRuntimeSchema, outputSchemas []configuration.NDCHttpRuntimeSchema) {
	t.Helper()

	assert.Equal(t, len(expectedSchemas), len(outputSchemas))
	for i, expected := range expectedSchemas {
		output := outputSchemas[i]
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
}

func assertDeepEqual(t *testing.T, expected any, reality any) {
	t.Helper()
	assert.DeepEqual(t,
		expected, reality,
		cmpopts.IgnoreUnexported(schema.ServerConfig{}, schema.NDCHttpSettings{}),
		cmp.Exporter(func(t reflect.Type) bool { return true }),
	)
}
