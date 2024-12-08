package internal

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gotest.tools/v3/assert"
)

func TestEvalURLAndHeaderParameters(t *testing.T) {
	testCases := []struct {
		name         string
		rawArguments string
		expectedURL  string
		errorMsg     string
		headers      map[string]string
	}{
		{
			name: "findPetsByStatus",
			rawArguments: `{
				"status": "available"
			}`,
			expectedURL: "/pet/findByStatus?status=available",
		},
		{
			name: "GetInvoices",
			rawArguments: `{
				"collection_method": "charge_automatically",
				"created": null,
				"customer": "UFGkQ6qKPc",
				"due_date": null,
				"ending_before": "bAOW2sHpAG",
				"expand": ["HbZr0T5gf8"],
				"limit": 19522,
				"starting_after": "McghIoX8E7",
				"status": "draft",
				"subscription": "UpqQmfokoF"
			}`,
			expectedURL: "/v1/invoices?collection_method=charge_automatically&customer=UFGkQ6qKPc&ending_before=bAOW2sHpAG&expand[]=HbZr0T5gf8&limit=19522&starting_after=McghIoX8E7&status=draft&subscription=UpqQmfokoF",
		},
	}

	ndcSchema := createMockSchema(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var info *rest.OperationInfo
			for key, f := range ndcSchema.Functions {
				if key == tc.name {
					info = &f
					break
				}
			}
			var arguments map[string]any
			assert.NilError(t, json.Unmarshal([]byte(tc.rawArguments), &arguments))

			builder := RequestBuilder{
				Schema:    ndcSchema,
				Operation: info,
				Arguments: arguments,
			}
			result, headers, err := builder.evalURLAndHeaderParameters()
			if tc.errorMsg != "" {
				assert.ErrorContains(t, err, tc.errorMsg)
			} else {
				assert.NilError(t, err)
				decodedValue, err := url.QueryUnescape(result.String())
				assert.NilError(t, err)
				assert.Equal(t, tc.expectedURL, decodedValue)
				for k, v := range tc.headers {
					assert.Equal(t, v, headers.Get(k))
				}
			}
		})
	}
}

func createMockSchema(t *testing.T) *rest.NDCHttpSchema {
	var ndcSchema rest.NDCHttpSchema
	rawSchemaBytes, err := os.ReadFile("../../ndc-http-schema/openapi/testdata/petstore3/expected.json")
	assert.NilError(t, err)
	assert.NilError(t, json.Unmarshal(rawSchemaBytes, &ndcSchema))

	return &ndcSchema
}
