package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeDataURI(t *testing.T) {
	testCases := []struct {
		input    string
		expected DataURI
		errorMsg string
	}{
		{
			input: "data:image/png;a=b;base64,aGVsbG8gd29ybGQ=",
			expected: DataURI{
				MediaType: "image/png",
				Parameters: map[string]string{
					"a": "b",
				},
				Data: "hello world",
			},
		},
		{
			input: "data:text/plain,hello_world",
			expected: DataURI{
				MediaType:  "text/plain",
				Data:       "hello_world",
				Parameters: map[string]string{},
			},
		},
		{
			input: "data:text/plain;ascii,hello_world",
			expected: DataURI{
				MediaType:  "text/plain",
				Data:       "hello_world",
				Parameters: map[string]string{},
			},
		},
		{
			input: "aGVsbG8gd29ybGQ=",
			expected: DataURI{
				Data: "hello world",
			},
		},
		{
			input:    "aadawdda ada",
			errorMsg: "illegal base64 data at input byte",
		},
		{
			input:    "data:text/plain",
			errorMsg: "invalid data uri",
		},
		{
			input:    "data:image/png;a=b;base64, test =",
			errorMsg: "illegal base64 data at input",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			data, err := DecodeDataURI(tc.input)

			if tc.errorMsg != "" {
				assert.ErrorContains(t, err, tc.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, *data)
			}
		})
	}
}
