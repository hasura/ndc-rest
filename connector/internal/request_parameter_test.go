package internal

import (
	"encoding/json"
	"net/url"
	"testing"

	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"gotest.tools/v3/assert"
)

func TestEvalQueryParameterURL(t *testing.T) {
	testCases := []struct {
		name     string
		param    *rest.RequestParameter
		keys     []Key
		values   []string
		expected string
	}{
		{
			name:     "empty",
			param:    &rest.RequestParameter{},
			keys:     []Key{NewKey("")},
			values:   []string{},
			expected: "",
		},
		{
			name: "form_explode_single",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []Key{},
			values:   []string{"3"},
			expected: "id=3",
		},
		{
			name: "form_single",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []Key{NewKey("")},
			values:   []string{"3"},
			expected: "id=3",
		},
		{
			name: "form_explode_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []Key{NewKey("")},
			values:   []string{"3", "4", "5"},
			expected: "id=3&id=4&id=5",
		},
		{
			name: "spaceDelimited_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStyleSpaceDelimited,
				},
			},
			keys:     []Key{NewKey("")},
			values:   []string{"3", "4", "5"},
			expected: "id=3 4 5",
		},
		{
			name: "spaceDelimited_explode_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleSpaceDelimited,
				},
			},
			keys:     []Key{NewKey("")},
			values:   []string{"3", "4", "5"},
			expected: "id=3&id=4&id=5",
		},

		{
			name: "pipeDelimited_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStylePipeDelimited,
				},
			},
			keys:     []Key{NewKey("")},
			values:   []string{"3", "4", "5"},
			expected: "id=3|4|5",
		},
		{
			name: "pipeDelimited_explode_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStylePipeDelimited,
				},
			},
			keys:     []Key{NewKey("")},
			values:   []string{"3", "4", "5"},
			expected: "id=3&id=4&id=5",
		},
		{
			name: "deepObject_explode_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleDeepObject,
				},
			},
			keys:     []Key{NewKey("")},
			values:   []string{"3", "4", "5"},
			expected: "id[]=3&id[]=4&id[]=5",
		},
		{
			name: "form_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []Key{NewKey("role")},
			values:   []string{"admin"},
			expected: "id=role,admin",
		},
		{
			name: "form_explode_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []Key{NewKey("role")},
			values:   []string{"admin"},
			expected: "role=admin",
		},
		{
			name: "deepObject_explode_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleDeepObject,
				},
			},
			keys:     []Key{NewKey("role")},
			values:   []string{"admin"},
			expected: "id[role]=admin",
		},
		{
			name: "form_array_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
			values:   []string{"admin"},
			expected: "id=role[][user],admin",
		},
		{
			name: "form_explode_array_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
			values:   []string{"admin"},
			expected: "role[][user]=admin",
		},
		{
			name: "form_explode_array_object_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
			values:   []string{"admin", "anonymous"},
			expected: "id[role][][user]=admin&id[role][][user]=anonymous",
		},
		{
			name: "deepObject_explode_array_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleDeepObject,
				},
			},
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
			values:   []string{"admin"},
			expected: "id[role][][user][]=admin",
		},
		{
			name: "deepObject_explode_array_object_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleDeepObject,
				},
			},
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
			values:   []string{"admin", "anonymous"},
			expected: "id[role][][user][]=admin&id[role][][user][]=anonymous",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			qValues := make(url.Values)
			evalQueryParameterURL(&qValues, tc.param.Name, tc.param.EncodingObject, tc.keys, tc.values)
			assert.Equal(t, tc.expected, encodeQueryValues(qValues, true))
		})
	}
}

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
				decodedValue, err := url.QueryUnescape(result)
				assert.NilError(t, err)
				assert.Equal(t, tc.expectedURL, decodedValue)
				for k, v := range tc.headers {
					assert.Equal(t, v, headers.Get(k))
				}
			}
		})
	}
}
