package command

import (
	"fmt"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestJson2Yaml(t *testing.T) {

	testCases := []struct {
		name     string
		filePath string
		noOutput bool
		errorMsg string
	}{
		{
			name:     "file_not_found",
			filePath: "foo.json",
			errorMsg: "failed to read content from foo.json: open foo.json: no such file or directory",
		},
		{
			name:     "success",
			filePath: "../openapi/testdata/petstore3/source.json",
		},
		{
			name:     "no_output",
			filePath: "../openapi/testdata/petstore2/swagger.json",
			noOutput: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var outputFilePath string
			if !tc.noOutput {
				tempDir := t.TempDir()
				outputFilePath = fmt.Sprintf("%s/output.yaml", tempDir)
			}

			err := Json2Yaml(&Json2YamlCommandArguments{
				File:   tc.filePath,
				Output: outputFilePath,
			}, nopLogger)

			if tc.errorMsg != "" {
				assertError(t, err, tc.errorMsg)
				return
			}

			assertNoError(t, err)

			if tc.noOutput {
				return
			}
			outputBytes, err := os.ReadFile(outputFilePath)
			if err != nil {
				t.Errorf("cannot read the output file at %s", outputFilePath)
				t.FailNow()
			}
			var output any
			if err := yaml.Unmarshal(outputBytes, &output); err != nil {
				t.Errorf("cannot decode the output file yaml at %s", outputFilePath)
				t.FailNow()
			}
		})
	}
}
