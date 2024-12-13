package internal

import (
	"encoding/json"
	"io"
	"net/url"
	"os"
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gotest.tools/v3/assert"
)

func TestQueryEvalURLAndHeaderParameters(t *testing.T) {
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

func TestMutationEvalURLAndHeaderParameters(t *testing.T) {
	testCases := []struct {
		name         string
		rawArguments string
		expectedURL  string
		expectedBody string
		errorMsg     string
		headers      map[string]string
	}{
		{
			name: "PostBillingMeterEvents",
			rawArguments: `{
        "body": {
          "event_name": "k8hAOi2B52",
          "identifier": "identifier_123",
          "payload": {
            "value": "25",
            "stripe_customer_id": "cus_NciAYcXfLnqBoz"
          },
          "timestamp": 931468280
        }
			}`,
			expectedURL:  "/v1/billing/meter_events",
			expectedBody: "event_name=k8hAOi2B52&identifier=identifier_123&payload[value]=25&payload[stripe_customer_id]=cus_NciAYcXfLnqBoz&timestamp=931468280",
		},
	}

	ndcSchema := createMockSchema(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var info *rest.OperationInfo
			for key, f := range ndcSchema.Procedures {
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

			result, err := builder.Build()
			if tc.errorMsg != "" {
				assert.ErrorContains(t, err, tc.errorMsg)

				return
			}

			assert.NilError(t, err)
			decodedValue, err := url.QueryUnescape(result.URL.String())
			assert.NilError(t, err)
			assert.Equal(t, tc.expectedURL, decodedValue)

			for k, v := range tc.headers {
				assert.Equal(t, v, result.Headers.Get(k))
			}

			bodyBytes, err := io.ReadAll(result.Body)
			assert.NilError(t, err)
			expected, err := url.ParseQuery(tc.expectedBody)
			assert.NilError(t, err)
			body, err := url.ParseQuery(string(bodyBytes))
			assert.NilError(t, err)

			assert.DeepEqual(t, expected, body)
		})
	}
}

func TestEvalURLAndHeaderParametersOAS2(t *testing.T) {
	testCases := []struct {
		name         string
		rawArguments string
		expectedURL  string
		errorMsg     string
		headers      map[string]string
	}{
		{
			name: "get_subject",
			rawArguments: `{
				"identifier": "thesauri/material/AAT.11914"
			}`,
			expectedURL: "/id/thesauri/material/AAT.11914",
		},
	}

	ndcSchema := createMockSchemaOAS2(t)
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

func createMockSchemaOAS2(t *testing.T) *rest.NDCHttpSchema {
	var ndcSchema rest.NDCHttpSchema
	rawSchemaBytes, err := os.ReadFile("../../ndc-http-schema/openapi/testdata/petstore2/expected.json")
	assert.NilError(t, err)
	assert.NilError(t, json.Unmarshal(rawSchemaBytes, &ndcSchema))

	return &ndcSchema
}
