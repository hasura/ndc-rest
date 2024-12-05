package contenttype

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gotest.tools/v3/assert"
)

func TestCreateMultipartForm(t *testing.T) {
	testCases := []struct {
		Name            string
		RawArguments    string
		Expected        map[string]string
		ExpectedHeaders map[string]http.Header
	}{
		{
			Name: "PostFiles",
			RawArguments: `{
		    "body": {
		      "expand": ["foo"],
		      "expand_json": ["foo","bar"],
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
				"expand[]":                  `foo`,
				"expand_json":               `["foo","bar"]`,
				"file":                      "hello world",
				"file_link_data.create":     "true",
				"file_link_data.expires_at": "181320689",
				"purpose":                   "business_icon",
			},
			ExpectedHeaders: map[string]http.Header{
				"expand[]": {
					"Content-Type": []string{rest.ContentTypeTextPlain},
				},
				"expand_json": {
					"Content-Type": []string{"application/json"},
				},
				"file": {
					"Content-Type":       []string{rest.ContentTypeOctetStream},
					"X-Rate-Limit-Limit": []string{"10"},
				},
			},
		},
		{
			Name: "uploadPetMultipart",
			RawArguments: `{
        "body": {
          "address": {
            "street": "street 1",
            "location": [0, 1]
          }
        }
      }`,
			Expected: map[string]string{
				"address": `{"location":[0,1],"street":"street 1"}`,
			},
			ExpectedHeaders: map[string]http.Header{},
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
			builder := NewMultipartFormEncoder(ndcSchema, info, arguments)
			buf, mediaType, err := builder.Encode(arguments["body"])
			assert.NilError(t, err)

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
