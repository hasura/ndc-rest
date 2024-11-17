package internal

import (
	"encoding/json"
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gotest.tools/v3/assert"
)

func TestCreateXMLForm(t *testing.T) {
	testCases := []struct {
		Name         string
		RawArguments string
		Expected     string
	}{
		{
			Name: "putPetXml",
			RawArguments: `{
				"body": {
					"id": 10,
					"name": "doggie",
					"category": {
						"id": 1,
						"name": "Dogs"
					},
					"photoUrls": [
						"string"
					],
					"tags": [
						{
							"id": 0,
							"name": "string"
						}
					],
					"status": "available"
				}
			}`,
			Expected: "<pet><category><id>1</id><name>Dogs</name></category><id>10</id><name>doggie</name><photoUrls><photoUrl>string</photoUrl></photoUrls><status>available</status><tags><tag><id>0</id><name>string</name></tag></tags></pet>",
		},
	}

	ndcSchema := createMockSchema(t)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			var info *rest.OperationInfo
			for key, f := range ndcSchema.Procedures {
				if key == tc.Name {
					info = &f
					break
				}
			}
			assert.Assert(t, info != nil)
			var arguments map[string]any
			assert.NilError(t, json.Unmarshal([]byte(tc.RawArguments), &arguments))
			argumentInfo := info.Arguments["body"]
			builder := RequestBuilder{
				Schema:    ndcSchema,
				Operation: info,
				Arguments: arguments,
			}
			result, err := builder.createXMLBody(&argumentInfo, arguments["body"])
			assert.NilError(t, err)
			assert.DeepEqual(t, tc.Expected, string(result))
		})
	}
}
