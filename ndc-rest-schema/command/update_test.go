package command

import (
	"encoding/json"
	"log/slog"
	"os"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hasura/ndc-rest/ndc-rest-schema/configuration"
	"github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"gotest.tools/v3/assert"
)

func TestUpdateCommand(t *testing.T) {
	testCases := []struct {
		Argument UpdateCommandArguments
		Expected string
	}{
		// go run ./ndc-rest-schema update -d ./ndc-rest-schema/command/testdata/patch
		{
			Argument: UpdateCommandArguments{
				Dir: "testdata/patch",
			},
			Expected: "testdata/patch/expected.json",
		},
		// go run ./ndc-rest-schema update -d ./ndc-rest-schema/command/testdata/auth
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

func readRuntimeSchemaFile(t *testing.T, filePath string) []configuration.NDCRestRuntimeSchema {
	t.Helper()
	rawBytes, err := os.ReadFile(filePath)
	assert.NilError(t, err)

	var result []configuration.NDCRestRuntimeSchema
	assert.NilError(t, json.Unmarshal(rawBytes, &result))

	return result
}

func assertSchemaEqual(t *testing.T, expectedSchemas []configuration.NDCRestRuntimeSchema, outputSchemas []configuration.NDCRestRuntimeSchema) {
	t.Helper()

	assert.Equal(t, len(expectedSchemas), len(outputSchemas))
	for i, expected := range expectedSchemas {
		output := outputSchemas[i]
		assetDeepEqual(t, expected.Settings.Headers, output.Settings.Headers)
		assetDeepEqual(t, expected.Settings.Security, output.Settings.Security)
		assetDeepEqual(t, expected.Settings.SecuritySchemes, output.Settings.SecuritySchemes)
		assetDeepEqual(t, expected.Settings.Version, output.Settings.Version)
		for i, server := range expected.Settings.Servers {
			sv := output.Settings.Servers[i]
			assetDeepEqual(t, server.Headers, sv.Headers)
			assetDeepEqual(t, server.ID, sv.ID)
			assetDeepEqual(t, server.Security, sv.Security)
			assetDeepEqual(t, server.SecuritySchemes, sv.SecuritySchemes)
			assetDeepEqual(t, server.URL, sv.URL)
			assetDeepEqual(t, server.TLS, sv.TLS)
		}
		assetDeepEqual(t, expected.ScalarTypes, output.ScalarTypes)
		objectBs, _ := json.Marshal(output.ObjectTypes)
		var objectTypes map[string]schema.ObjectType
		assert.NilError(t, json.Unmarshal(objectBs, &objectTypes))
		assetDeepEqual(t, expected.ObjectTypes, objectTypes)
		assetDeepEqual(t, expected.Procedures, output.Procedures)
		assetDeepEqual(t, expected.Functions, output.Functions)
	}
}

func assetDeepEqual(t *testing.T, expected any, reality any) {
	t.Helper()
	assert.DeepEqual(t,
		expected, reality,
		cmpopts.IgnoreUnexported(schema.ServerConfig{}, schema.NDCRestSettings{}),
		cmp.Exporter(func(t reflect.Type) bool { return true }),
	)
}
