package schema

import (
	"encoding/json"
	"testing"
)

func TestNDCRestSettings(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected NDCRestSettings
	}{
		{
			name: "setting_success",
			input: `{
				"servers": [
					{
						"url": "{{PET_STORE_SERVER_URL:-https://petstore3.swagger.io/api/v3}}"
					},
					{
						"url": "https://petstore3.swagger.io/api/v3.1"
					}
				],
				"securitySchemes": {
					"api_key": {
						"type": "apiKey",
						"value": "{{PET_STORE_API_KEY}}",
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
				"timeout": "{{PET_STORE_TIMEOUT}}",
				"retry": {
					"times": "{{PET_STORE_RETRY_TIMES}}",
					"delay": 1000,
					"httpStatus": "{{PET_STORE_RETRY_HTTP_STATUS}}"
				},
				"security": [
					{},
					{
						"petstore_auth": ["write:pets", "read:pets"]
					}
				],
				"version": "1.0.19"
			}`,
			expected: NDCRestSettings{
				Servers: []ServerConfig{
					{
						URL: *NewEnvStringTemplate(NewEnvTemplateWithDefault("PET_STORE_SERVER_URL", "https://petstore3.swagger.io/api/v3")),
					},
					{
						URL: *EnvString{}.WithValue("https://petstore3.swagger.io/api/v3.1"),
					},
				},
				SecuritySchemes: map[string]SecurityScheme{
					"api_key": {
						Type:  APIKeyScheme,
						Value: NewEnvStringTemplate(NewEnvTemplate("PET_STORE_API_KEY")),
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
				Timeout: NewEnvIntTemplate(NewEnvTemplate("PET_STORE_TIMEOUT")),
				Retry: &RetryPolicySetting{
					Times:      *NewEnvIntTemplate(NewEnvTemplate("PET_STORE_RETRY_TIMES")),
					Delay:      *NewEnvIntValue(1000),
					HTTPStatus: *NewEnvIntsTemplate(NewEnvTemplate("PET_STORE_RETRY_HTTP_STATUS")),
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
			var result NDCRestSettings
			if err := json.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Errorf("failed to decode: %s", err)
				t.FailNow()
			}
			for i, s := range tc.expected.Servers {
				assertDeepEqual(t, s.URL.String(), result.Servers[i].URL.String())
			}
			assertDeepEqual(t, tc.expected.Headers, result.Headers)
			assertDeepEqual(t, tc.expected.Retry, result.Retry)
			assertDeepEqual(t, tc.expected.Security, result.Security)
			assertDeepEqual(t, tc.expected.SecuritySchemes, result.SecuritySchemes)
			assertDeepEqual(t, tc.expected.Timeout, result.Timeout)
			assertDeepEqual(t, tc.expected.Version, result.Version)

			_, err := json.Marshal(tc.expected)
			if err != nil {
				t.Errorf("failed to encode: %s", err)
				t.FailNow()
			}
		})
	}
}
