package rest

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"testing"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"gotest.tools/v3/assert"
)

func TestCreateMultipartForm(t *testing.T) {
	testCases := []struct {
		Name            string
		RawBody         string
		RawArguments    string
		Expected        map[string]string
		ExpectedHeaders map[string]http.Header
	}{
		{
			Name: "multiple_fields",
			RawBody: `{
				"contentType": "multipart/form-data",
				"schema": {
					"type": "object",
					"properties": {
						"expand": {
							"type": "array",
							"nullable": true,
							"items": {
								"type": "String",
								"maxLength": 5000
							}
						},
						"expand_expose": {
							"type": "array",
							"nullable": true,
							"items": {
								"type": "String",
								"maxLength": 5000
							}
						},
						"file": {
							"type": "Binary"
						},
						"file_link_data": {
							"type": "object",
							"nullable": true,
							"properties": {
								"create": {
									"type": "Boolean"
								},
								"expires_at": {
									"type": "UnixTime",
									"nullable": true
								}
							}
						},
						"purpose": {
							"type": "PostFilesBodyPurpose"
						}
					}
				},
				"encoding": {
					"expand_expose": {
						"style": "deepObject",
						"explode": true
					},
					"file_link_data": {
						"style": "deepObject",
						"explode": true
					},
					"file": {
						"headers": {
							"X-Rate-Limit-Limit": {
								"argumentName": "headerXRateLimitLimit",
								"schema": {
									"type": "integer"
								}
							}
						}
					}
				}
			}`,
			RawArguments: `{
        "body": {
          "expand": ["foo", "bar"],
          "expand_expose": ["foo"],
          "file": "aGVsbG8gd29ybGQ=",
          "file_link_data": {
            "create": true,
            "expires_at": 181320689
          },
          "purpose": "business_icon"
        },
				"headerXRateLimitLimit": 10
      }`,
			Expected: map[string]string{
				"expand":                    `["foo","bar"]`,
				"expand_expose[]":           `foo`,
				"file":                      "hello world",
				"file_link_data.create":     "true",
				"file_link_data.expires_at": "181320689",
				"purpose":                   "business_icon",
			},
			ExpectedHeaders: map[string]http.Header{
				"expand": {
					"Content-Type": []string{"application/json"},
				},
				"file": {
					"X-Rate-Limit-Limit": []string{"10"},
				},
			},
		},
	}

	rc := &RESTConnector{
		schema: &schema.SchemaResponse{
			ScalarTypes: schema.SchemaResponseScalarTypes{
				"PostFilesBodyPurpose": schema.ScalarType{
					AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
					ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
					Representation: schema.NewTypeRepresentationEnum([]string{
						"account_requirement",
						"additional_verification",
						"business_icon",
						"business_logo",
						"customer_signature",
						"dispute_evidence",
						"identity_document",
						"pci_document",
						"tax_document_user_upload",
						"terminal_reader_splashscreen",
					}).Encode(),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			var reqBody rest.RequestBody
			var arguments map[string]any
			assert.NilError(t, json.Unmarshal([]byte(tc.RawBody), &reqBody))
			assert.NilError(t, json.Unmarshal([]byte(tc.RawArguments), &arguments))

			buf, mediaType, err := rc.createMultipartForm(&reqBody, arguments)
			assert.NilError(t, err)

			// log.Println("form data:", string(buf.String()))
			_, params, err := mime.ParseMediaType(mediaType)
			assert.NilError(t, err)

			reader := multipart.NewReader(buf, params["boundary"])
			var count int
			results := make(map[string]string)
			for {
				form, err := reader.NextPart()
				if err != nil && strings.Contains(err.Error(), io.EOF.Error()) {
					break
				}
				assert.NilError(t, err)
				count++
				name := form.FormName()
				expected, ok := tc.Expected[name]
				if !ok {
					t.Fatalf("field %s does not exist", name)
				} else {
					result, err := io.ReadAll(form)
					assert.NilError(t, err)
					assert.Equal(t, expected, string(result))
					results[name] = string(result)
					expectedHeader := tc.ExpectedHeaders[name]

					for key, value := range expectedHeader {
						assert.DeepEqual(t, value, form.Header[key])
					}
				}
			}
			if len(tc.Expected) != count {
				assert.DeepEqual(t, tc.Expected, results)
			}
		})
	}
}

func TestCreateFormURLEncoded(t *testing.T) {
	testCases := []struct {
		Name         string
		RawBody      string
		RawArguments string
		Expected     string
	}{
		{
			Name: "multiple_fields",
			RawBody: `{
				"contentType": "application/x-www-form-urlencoded",
				"schema": {
					"type": "object",
					"properties": {
						"expand": {
							"type": "array",
							"nullable": true,
							"items": {
								"type": "String",
								"maxLength": 5000
							}
						},
						"expand_expose": {
							"type": "array",
							"nullable": true,
							"items": {
								"type": "String",
								"maxLength": 5000
							}
						},
						"file": {
							"type": "Binary"
						},
						"file_link_data": {
							"type": "object",
							"nullable": true,
							"properties": {
								"create": {
									"type": "Boolean"
								},
								"expires_at": {
									"type": "UnixTime",
									"nullable": true
								}
							}
						},
						"purpose": {
							"type": "PostFilesBodyPurpose"
						}
					}
				},
				"encoding": {
					"expand_expose": {
						"style": "deepObject",
						"explode": true
					},
					"file_link_data": {
						"style": "deepObject",
						"explode": true
					},
					"file": {
						"headers": {
							"X-Rate-Limit-Limit": {
								"argumentName": "headerXRateLimitLimit",
								"schema": {
									"type": "integer"
								}
							}
						}
					}
				}
			}`,
			RawArguments: `{
        "body": {
          "expand": ["foo", "bar"],
          "expand_expose": ["foo"],
          "file": "aGVsbG8gd29ybGQ=",
          "file_link_data": {
            "create": true,
            "expires_at": 181320689
          },
          "purpose": "business_icon"
        },
				"headerXRateLimitLimit": 10
      }`,
			Expected: "expand_expose[]=foo&file=aGVsbG8gd29ybGQ=&file_link_data[create]=true&file_link_data[expires_at]=181320689&purpose=business_icon&expand=foo&expand=bar",
		},
	}

	rc := &RESTConnector{
		schema: &schema.SchemaResponse{
			ScalarTypes: schema.SchemaResponseScalarTypes{
				"PostFilesBodyPurpose": schema.ScalarType{
					AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
					ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
					Representation: schema.NewTypeRepresentationEnum([]string{
						"account_requirement",
						"additional_verification",
						"business_icon",
						"business_logo",
						"customer_signature",
						"dispute_evidence",
						"identity_document",
						"pci_document",
						"tax_document_user_upload",
						"terminal_reader_splashscreen",
					}).Encode(),
				},
			},
		},
	}

	parseQueryAndSort := func(input string) []string {
		items := strings.Split(input, "&")
		slices.Sort(items)
		return items
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			var reqBody rest.RequestBody
			var arguments map[string]any
			assert.NilError(t, json.Unmarshal([]byte(tc.RawBody), &reqBody))
			assert.NilError(t, json.Unmarshal([]byte(tc.RawArguments), &arguments))

			buf, err := rc.createFormURLEncoded(&reqBody, arguments["body"])
			assert.NilError(t, err)
			result, err := io.ReadAll(buf)
			assert.NilError(t, err)
			assert.DeepEqual(t, parseQueryAndSort(tc.Expected), parseQueryAndSort(string(result)))
		})
	}
}
