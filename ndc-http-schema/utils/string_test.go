package utils

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestRemoveYAMLSpecialCharacters(t *testing.T) {
	testCases := []struct {
		Input    string
		Expected string
	}{
		{
			Input:    "\b\t\u0009Some\u0000thing\\u0002",
			Expected: "  Something",
		},
		{
			Input:    "^[a-zA-Z\\u0080-\\u024F\\s\\/\\-\\)\\(\\`\\.\\\"\\']+$",
			Expected: "^[a-zA-Z-\\s\\/\\-\\)\\(\\`\\.\\\"\\']+$",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Expected, func(t *testing.T) {
			assert.Equal(t, tc.Expected, string(RemoveYAMLSpecialCharacters([]byte(tc.Input))))
		})
	}
}
