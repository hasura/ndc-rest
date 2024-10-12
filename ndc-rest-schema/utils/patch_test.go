package utils

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPatch(t *testing.T) {
	testCases := []struct {
		Name         string
		InputPath    string
		Patches      []PatchConfig
		ExpectedPath string
	}{
		{
			Name:         "basic",
			InputPath:    "testdata/patch/source.json",
			ExpectedPath: "testdata/patch/expected.json",
			Patches: []PatchConfig{
				{
					Path: "testdata/patch/patches",
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {

			input, err := ReadFileFromPath(tc.InputPath)
			if err != nil {
				t.Fatalf("failed to read input file: %s", err)
			}
			output, err := ReadFileFromPath(tc.ExpectedPath)
			if err != nil {
				t.Fatalf("failed to read output file: %s", err)
			}
			result, err := ApplyPatch(input, tc.Patches)
			if err != nil {
				t.Fatalf("failed to apply patches: %s", err)
			}
			var jResult, jExpected map[string]any
			if err := json.Unmarshal(result, &jResult); err != nil {
				t.Fatalf("failed to decode result json: %s", err)
			}
			if err := json.Unmarshal(output, &jExpected); err != nil {
				t.Fatalf("failed to decode expected json: %s", err)
			}
			for k, v := range jExpected {
				if !reflect.DeepEqual(v, jResult[k]) {
					eb, _ := json.Marshal(v)
					rb, _ := json.Marshal(jResult[k])

					t.Fatalf("field %s does not equal\nexpected: %v\ngot     : %v", k, string(eb), string(rb))
				}
			}
		})
	}
}
