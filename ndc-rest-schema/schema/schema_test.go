package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hasura/ndc-sdk-go/schema"
)

func assertDeepEqual(_ *testing.T, expected any, reality any, msgs ...string) {
	if !reflect.DeepEqual(expected, reality) {
		panic(fmt.Errorf("%s: not equal\nexpected: %+v\ngot     : %+v", strings.Join(msgs, " "), expected, reality))
	}
}

func TestDecodeRESTProcedureInfo(t *testing.T) {
	testCases := []struct {
		name     string
		raw      string
		expected RESTProcedureInfo
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
			expected: RESTProcedureInfo{
				Request: &Request{
					URL:    "/pets",
					Method: "post",
				},
				ProcedureInfo: schema.ProcedureInfo{
					Arguments:   make(schema.ProcedureInfoArguments),
					Description: toPtr("Create a pet"),
					Name:        "createPets",
					ResultType:  schema.NewNullableNamedType("Boolean").Encode(),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var procedure RESTProcedureInfo
			if err := json.Unmarshal([]byte(tc.raw), &procedure); err != nil {
				t.Errorf("failed to unmarshal: %s", err)
				t.FailNow()
			}
			assertDeepEqual(t, tc.expected, procedure)
			assertDeepEqual(t, tc.expected.Request.Clone(), procedure.Request.Clone())
		})
	}
}

func TestDecodeRESTFunctionInfo(t *testing.T) {
	testCases := []struct {
		name     string
		raw      string
		expected RESTFunctionInfo
	}{
		{
			name: "success",
			raw: ` {
				"request": {
					"url": "/pets",
					"method": "get",
					"parameters": [
						{
							"name": "limit",
							"in": "query",
							"schema": { "type": "integer", "maximum": 100, "format": "int32", "nullable": true }
						}
					]
				},
				"arguments": {
					"limit": {
						"description": "How many items to return at one time (max 100)",
						"type": {
							"type": "nullable",
							"underlying_type": { "name": "Int", "type": "named" }
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
			expected: RESTFunctionInfo{
				Request: &Request{
					URL:    "/pets",
					Method: "get",
					Parameters: []RequestParameter{
						{
							Name: "limit",
							In:   "query",
							Schema: &TypeSchema{
								Type:     "integer",
								Maximum:  toPtr(float64(100)),
								Format:   "int32",
								Nullable: true,
							},
						},
					},
				},
				FunctionInfo: schema.FunctionInfo{
					Arguments: schema.FunctionInfoArguments{
						"limit": schema.ArgumentInfo{
							Description: toPtr("How many items to return at one time (max 100)"),
							Type:        schema.NewNullableNamedType("Int").Encode(),
						},
					},
					Description: toPtr("List all pets"),
					Name:        "listPets",
					ResultType:  schema.NewArrayType(schema.NewNamedType("Pet")).Encode(),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fn RESTFunctionInfo
			if err := json.Unmarshal([]byte(tc.raw), &fn); err != nil {
				t.Errorf("failed to unmarshal: %s", err)
				t.FailNow()
			}
			assertDeepEqual(t, tc.expected, fn)
			assertDeepEqual(t, tc.expected.Request.Clone(), fn.Request.Clone())

		})
	}
}
