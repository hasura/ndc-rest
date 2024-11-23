package internal

import (
	"bytes"
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gotest.tools/v3/assert"
)

func TestCreateXMLForm(t *testing.T) {
	testCases := []struct {
		Name string
		Body map[string]any

		Expected string
	}{
		{
			Name: "putPetXml",
			Body: map[string]any{
				"id":   int64(10),
				"name": "doggie",
				"category": map[string]any{
					"id":   int64(1),
					"name": "Dogs",
				},
				"photoUrls": []any{"string"},
				"tags": []any{
					map[string]any{
						"id":   int64(0),
						"name": "string",
					},
				},
				"status": "available",
			},
			Expected: "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<pet><category><id>1</id><name>Dogs</name></category><id>10</id><name>doggie</name><photoUrls><photoUrl>string</photoUrl></photoUrls><status>available</status><tags><tag><id>0</id><name>string</name></tag></tags></pet>",
		},
		{
			Name: "putCommentXml",
			Body: map[string]any{
				"user": "Iggy",
				"comment": []any{
					map[string]any{
						"who":       "Iggy",
						"when":      "2021-10-15 13:28:22 UTC",
						"id":        int64(1),
						"bsrequest": int64(115),
						"xmlValue":  "This is a pretty cool request!",
					},
					map[string]any{
						"who":      "Iggy",
						"when":     "2021-10-15 13:49:39 UTC",
						"id":       int64(2),
						"project":  "home:Admin",
						"xmlValue": "This is a pretty cool project!",
					},
					map[string]any{
						"who":      "Iggy",
						"when":     "2021-10-15 13:54:38 UTC",
						"id":       int64(3),
						"project":  "home:Admin",
						"package":  "0ad",
						"xmlValue": "This is a pretty cool package!",
					},
				},
			},
			Expected: `<?xml version="1.0" encoding="UTF-8"?>
<comments user="Iggy"><comment bsrequest="115" id="1" when="2021-10-15 13:28:22 UTC" who="Iggy">This is a pretty cool request!</comment><comment id="2" project="home:Admin" when="2021-10-15 13:49:39 UTC" who="Iggy">This is a pretty cool project!</comment><comment id="3" package="0ad" project="home:Admin" when="2021-10-15 13:54:38 UTC" who="Iggy">This is a pretty cool package!</comment></comments>`,
		},
		{
			Name: "putBookXml",
			Body: map[string]any{
				"id":     int64(0),
				"title":  "string",
				"author": "Author",
				"attr":   "foo",
			},
			Expected: `<?xml version="1.0" encoding="UTF-8"?>
<smp:book smp:attr="foo" xmlns:smp="http://example.com/schema"><author>Author</author><id>0</id><title>string</title></smp:book>`,
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
			argumentInfo := info.Arguments["body"]
			result, err := NewXMLEncoder(ndcSchema).Encode(&argumentInfo, tc.Body)
			assert.NilError(t, err)
			assert.Equal(t, tc.Expected, string(result))

			dec := NewXMLDecoder(ndcSchema)
			parsedResult, err := dec.Decode(bytes.NewBuffer([]byte(tc.Expected)), info.ResultType)
			assert.NilError(t, err)

			assert.DeepEqual(t, tc.Body, parsedResult)
		})
	}
}
