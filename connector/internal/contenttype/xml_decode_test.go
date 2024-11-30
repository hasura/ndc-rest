package contenttype

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"gotest.tools/v3/assert"
)

func TestDecodeXML(t *testing.T) {
	testCases := []struct {
		Name     string
		Body     string
		Type     schema.Type
		Expected map[string]any
	}{
		{
			Name: "getSearchXml",
			Type: schema.NewNamedType("JSON").Encode(),
			Body: `<collection><project name="home:Admin"><title></title><description></description><person userid="Admin" role="maintainer"/><repository name="openSUSE_Tumbleweed"><path project="openSUSE.org:openSUSE:Factory" repository="snapshot"/><arch>x86_64</arch></repository><repository name="15.3"><path project="openSUSE.org:openSUSE:Leap:15.3" repository="standard"/><arch>x86_64</arch></repository></project><project name="openSUSE.org"><title>Standard OBS instance at build.opensuse.org</title><description>This instance delivers the default build targets for OBS.</description><remoteurl>https://api.opensuse.org/public</remoteurl></project></collection>`,
			Expected: map[string]any{
				"project": []any{
					map[string]any{
						"attributes":  map[string]string{"name": "home:Admin"},
						"description": string(""),
						"person": map[string]any{
							"attributes": map[string]string{"role": "maintainer", "userid": "Admin"},
							"content":    string(""),
						},
						"repository": []any{
							map[string]any{
								"arch":       string("x86_64"),
								"attributes": map[string]string{"name": "openSUSE_Tumbleweed"},
								"path": map[string]any{
									"attributes": map[string]string{"project": "openSUSE.org:openSUSE:Factory", "repository": "snapshot"},
									"content":    string(""),
								},
							},
							map[string]any{
								"arch":       string("x86_64"),
								"attributes": map[string]string{"name": "15.3"},
								"path": map[string]any{
									"attributes": map[string]string{"project": "openSUSE.org:openSUSE:Leap:15.3", "repository": "standard"},
									"content":    string(""),
								},
							},
						},
						"title": string(""),
					},
					map[string]any{
						"attributes":  map[string]string{"name": "openSUSE.org"},
						"description": string("This instance delivers the default build targets for OBS."),
						"remoteurl":   string("https://api.opensuse.org/public"),
						"title":       string("Standard OBS instance at build.opensuse.org"),
					},
				},
			},
		},
	}

	ndcSchema := createMockSchema(t)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result, err := NewXMLDecoder(ndcSchema).Decode(strings.NewReader(tc.Body), tc.Type)
			assert.NilError(t, err)
			assert.DeepEqual(t, tc.Expected, result)
		})
	}
}

func createMockSchema(t *testing.T) *rest.NDCHttpSchema {
	var ndcSchema rest.NDCHttpSchema
	rawSchemaBytes, err := os.ReadFile("../../../ndc-http-schema/openapi/testdata/petstore3/expected.json")
	assert.NilError(t, err)
	assert.NilError(t, json.Unmarshal(rawSchemaBytes, &ndcSchema))

	return &ndcSchema
}
