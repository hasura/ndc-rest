package schema

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hasura/ndc-sdk-go/utils"
	"gotest.tools/v3/assert"
)

func TestNDCHttpSettings(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected NDCHttpSettings
	}{
		{
			name: "setting_success",
			input: `{
				"servers": [
					{
						"url": {
							"env": "PET_STORE_SERVER_URL",
							"value": "https://petstore3.swagger.io/api/v3"
						}
					},
					{
						"url": {
							"value": "https://petstore3.swagger.io/api/v3.1"
						}
					}
				],
				"securitySchemes": {
					"api_key": {
						"type": "apiKey",
						"value": {
							"env": "PET_STORE_API_KEY"
						},
						"in": "header",
						"name": "api_key"
					},
					"petstore_auth": {
						"type": "oauth2",
						"flows": {
							"implicit": {
								"authorizationUrl": "https://petstore3.swagger.io/oauth/authorize",
								"scopes": {
									"read:pets": "read your pets",
									"write:pets": "modify pets in your account"
								}
							}
						}
					}
				},
				"security": [
					{},
					{
						"petstore_auth": ["write:pets", "read:pets"]
					}
				],
				"version": "1.0.19"
			}`,
			expected: NDCHttpSettings{
				Servers: []ServerConfig{
					{
						URL: utils.NewEnvString("PET_STORE_SERVER_URL", "https://petstore3.swagger.io/api/v3"),
					},
					{
						URL: utils.NewEnvStringValue("https://petstore3.swagger.io/api/v3.1"),
					},
				},
				SecuritySchemes: map[string]SecurityScheme{
					"api_key": {
						Type:  APIKeyScheme,
						Value: utils.ToPtr(utils.NewEnvStringVariable("PET_STORE_API_KEY")),
						APIKeyAuthConfig: &APIKeyAuthConfig{
							In:   APIKeyInHeader,
							Name: "api_key",
						},
					},
					"petstore_auth": {
						Type: OAuth2Scheme,
						OAuth2Config: &OAuth2Config{
							Flows: map[OAuthFlowType]OAuthFlow{
								ImplicitFlow: {
									AuthorizationURL: "https://petstore3.swagger.io/oauth/authorize",
									Scopes: map[string]string{
										"read:pets":  "read your pets",
										"write:pets": "modify pets in your account",
									},
								},
							},
						},
					},
				},
				Security: AuthSecurities{
					AuthSecurity{},
					NewAuthSecurity("petstore_auth", []string{"write:pets", "read:pets"}),
				},
				Version: "1.0.19",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result NDCHttpSettings
			if err := json.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Errorf("failed to decode: %s", err)
				t.FailNow()
			}
			for i, s := range tc.expected.Servers {
				assert.DeepEqual(t, s.URL.Variable, result.Servers[i].URL.Variable)
				assert.DeepEqual(t, s.URL.Value, result.Servers[i].URL.Value)
			}
			assert.DeepEqual(t, tc.expected.Headers, result.Headers)
			assert.DeepEqual(t, tc.expected.Security, result.Security)
			assert.DeepEqual(t, tc.expected.SecuritySchemes, result.SecuritySchemes, cmp.Exporter(func(t reflect.Type) bool { return true }))
			assert.DeepEqual(t, tc.expected.Version, result.Version)

			_, err := json.Marshal(tc.expected)
			if err != nil {
				t.Errorf("failed to encode: %s", err)
				t.FailNow()
			}
		})
	}
}
