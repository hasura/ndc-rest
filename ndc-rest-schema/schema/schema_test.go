package schema

import (
	"encoding/json"
	"testing"

	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"gotest.tools/v3/assert"
)

func TestDecodeRESTProcedureInfo(t *testing.T) {
	testCases := []struct {
		name     string
		raw      string
		expected OperationInfo
	}{
		{
			name: "success",
			raw: `{
				"request": { "url": "/pets", "method": "post" },
				"arguments": {},
				"description": "Create a pet",
				"name": "createPets",
				"result_type": {
					"type": "nullable",
					"underlying_type": { "name": "Boolean", "type": "named" }
				}
			}`,
			expected: OperationInfo{
				Request: &Request{
					URL:    "/pets",
					Method: "post",
				},
				Arguments:   map[string]ArgumentInfo{},
				Description: utils.ToPtr("Create a pet"),
				ResultType:  schema.NewNullableNamedType("Boolean").Encode(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var procedure OperationInfo
			if err := json.Unmarshal([]byte(tc.raw), &procedure); err != nil {
				t.Errorf("failed to unmarshal: %s", err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected, procedure)
			assert.DeepEqual(t, tc.expected.Request.Clone(), procedure.Request.Clone())
		})
	}
}

func TestDecodeRESTFunctionInfo(t *testing.T) {
	testCases := []struct {
		name     string
		raw      string
		expected OperationInfo
	}{
		{
			name: "success",
			raw: `{
				"request": {
					"url": "/pets",
					"method": "get"
				},
				"arguments": {
					"limit": {
						"description": "How many items to return at one time (max 100)",
						"type": {
							"type": "nullable",
							"underlying_type": { "name": "Int", "type": "named" }
						},
						"rest": {
							"name": "limit",
							"in": "query",
							"schema": { "type": ["integer"], "maximum": 100, "format": "int32", "nullable": true }
						}
					}
				},
				"description": "List all pets",
				"name": "listPets",
				"result_type": {
					"element_type": { "name": "Pet", "type": "named" },
					"type": "array"
				}
			}`,
			expected: OperationInfo{
				Request: &Request{
					URL:    "/pets",
					Method: "get",
				},
				Arguments: map[string]ArgumentInfo{
					"limit": {
						ArgumentInfo: schema.ArgumentInfo{
							Description: utils.ToPtr("How many items to return at one time (max 100)"),
							Type:        schema.NewNullableNamedType("Int").Encode(),
						},
						Rest: &RequestParameter{
							Name: "limit",
							In:   "query",
							Schema: &TypeSchema{
								Type:    []string{"integer"},
								Maximum: utils.ToPtr(float64(100)),
								Format:  "int32",
							},
						},
					},
				},
				Description: utils.ToPtr("List all pets"),
				ResultType:  schema.NewArrayType(schema.NewNamedType("Pet")).Encode(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fn OperationInfo
			if err := json.Unmarshal([]byte(tc.raw), &fn); err != nil {
				t.Errorf("failed to unmarshal: %s", err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected, fn)
			assert.DeepEqual(t, tc.expected.Request.Clone(), fn.Request.Clone())
		})
	}
}
