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
	t.Setenv("PET_STORE_API_KEY", "api_key")
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
						},
						"tls": {
							"certFile": {
								"env": "PET_STORE_CERT_FILE"
							},
							"certPem": {
								"env": "PET_STORE_CERT_PEM"
							},
							"keyFile": {
								"env": "PET_STORE_KEY_FILE"
							},
							"keyPem": {
								"env": "PET_STORE_KEY_PEM"
							},
							"caFile": {
								"env": "PET_STORE_CA_FILE"
							},
							"caPem": {
								"env": "PET_STORE_CA_PEM"
							},
							"insecureSkipVerify": {
								"env": "PET_STORE_INSECURE_SKIP_VERIFY",
								"value": true
							},
							"includeSystemCACertsPool": {
								"env": "PET_STORE_INCLUDE_SYSTEM_CA_CERT_POOL",
								"value": true
							},
							"serverName": {
								"env": "PET_STORE_SERVER_NAME"
							},
							"minVersion": "1.0",
							"maxVersion": "1.3",
							"cipherSuites": ["TLS_AES_128_GCM_SHA256"]
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
					"http": {
						"type": "http",
						"value": {
							"env": "PET_STORE_API_KEY"
						},
						"scheme": "bearer",
						"header": "Authorization"
					},
					"basic": {
						"type": "basic",
						"username": {
							"value": "user"
						},
						"password": {
							"value": "password"
						}
					},
					"cookie": {
						"type": "cookie"
					},
					"mutualTLS": {
						"type": "mutualTLS"
					},
					"oidc": {
						"type": "openIdConnect",
						"openIdConnectUrl": "http://localhost:8080/oauth/token"
					},
					"petstore_auth": {
						"type": "oauth2",
						"flows": {
							"implicit": {
								"authorizationUrl": "https://petstore3.swagger.io/oauth/authorize",
								"tokenUrl": {
									"value": "https://petstore3.swagger.io/oauth/token"
								},
								"refreshUrl": {
									"value": "https://petstore3.swagger.io/oauth/token",
									"env": "PET_STORE_AUTH_REFRESH_URL"
								},
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
				"version": "1.0.19",
				"tls": {
					"certFile": {
						"env": "PET_STORE_CERT_FILE"
					},
					"certPem": {
						"env": "PET_STORE_CERT_PEM"
					},
					"keyFile": {
						"env": "PET_STORE_KEY_FILE"
					},
					"keyPem": {
						"env": "PET_STORE_KEY_PEM"
					},
					"caFile": {
						"env": "PET_STORE_CA_FILE"
					},
					"caPem": {
						"env": "PET_STORE_CA_PEM"
					},
					"insecureSkipVerify": {
						"env": "PET_STORE_INSECURE_SKIP_VERIFY",
						"value": true
					},
					"includeSystemCACertsPool": {
						"env": "PET_STORE_INCLUDE_SYSTEM_CA_CERT_POOL",
						"value": true
					},
					"serverName": {
						"env": "PET_STORE_SERVER_NAME"
					},
					"minVersion": "1.0",
					"maxVersion": "1.3",
					"cipherSuites": ["TLS_AES_128_GCM_SHA256"]
				}
			}`,
			expected: NDCHttpSettings{
				Servers: []ServerConfig{
					{
						URL: utils.NewEnvString("PET_STORE_SERVER_URL", "https://petstore3.swagger.io/api/v3"),
					},
					{
						URL: utils.NewEnvStringValue("https://petstore3.swagger.io/api/v3.1"),
						TLS: &TLSConfig{
							CertFile: &utils.EnvString{
								Variable: utils.ToPtr("PET_STORE_CERT_FILE"),
							},
							CertPem: &utils.EnvString{
								Variable: utils.ToPtr("PET_STORE_CERT_PEM"),
							},
							KeyFile: &utils.EnvString{
								Variable: utils.ToPtr("PET_STORE_KEY_FILE"),
							},
							KeyPem: &utils.EnvString{
								Variable: utils.ToPtr("PET_STORE_KEY_PEM"),
							},
							CAFile: &utils.EnvString{
								Variable: utils.ToPtr("PET_STORE_CA_FILE"),
							},
							CAPem: &utils.EnvString{
								Variable: utils.ToPtr("PET_STORE_CA_PEM"),
							},
							InsecureSkipVerify: &utils.EnvBool{
								Variable: utils.ToPtr("PET_STORE_INSECURE_SKIP_VERIFY"),
								Value:    utils.ToPtr(true),
							},
							IncludeSystemCACertsPool: &utils.EnvBool{
								Variable: utils.ToPtr("PET_STORE_INCLUDE_SYSTEM_CA_CERT_POOL"),
								Value:    utils.ToPtr(true),
							},
							ServerName: &utils.EnvString{
								Variable: utils.ToPtr("PET_STORE_SERVER_NAME"),
							},
							MinVersion:   "1.0",
							MaxVersion:   "1.3",
							CipherSuites: []string{"TLS_AES_128_GCM_SHA256"},
						},
					},
				},
				SecuritySchemes: map[string]SecurityScheme{
					"api_key": {
						SecuritySchemer: &APIKeyAuthConfig{
							Type:  APIKeyScheme,
							In:    APIKeyInHeader,
							Name:  "api_key",
							Value: utils.NewEnvStringVariable("PET_STORE_API_KEY"),
						},
					},
					"basic": {
						SecuritySchemer: &BasicAuthConfig{
							Type:     BasicAuthScheme,
							Username: utils.NewEnvStringValue("user"),
							Password: utils.NewEnvStringValue("password"),
						},
					},
					"http": {
						SecuritySchemer: &HTTPAuthConfig{
							Type:   HTTPAuthScheme,
							Header: "Authorization",
							Scheme: "bearer",
							Value:  utils.NewEnvStringVariable("PET_STORE_API_KEY"),
						},
					},
					"cookie": {
						SecuritySchemer: NewCookieAuthConfig(),
					},
					"mutualTLS": {
						SecuritySchemer: NewMutualTLSAuthConfig(),
					},
					"oidc": {
						SecuritySchemer: NewOpenIDConnectConfig("http://localhost:8080/oauth/token"),
					},
					"petstore_auth": {
						SecuritySchemer: &OAuth2Config{
							Type: OAuth2Scheme,
							Flows: map[OAuthFlowType]OAuthFlow{
								ImplicitFlow: {
									AuthorizationURL: "https://petstore3.swagger.io/oauth/authorize",
									TokenURL:         utils.ToPtr(utils.NewEnvStringValue("https://petstore3.swagger.io/oauth/token")),
									RefreshURL:       utils.ToPtr(utils.NewEnvString("PET_STORE_AUTH_REFRESH_URL", "https://petstore3.swagger.io/oauth/token")),
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
				TLS: &TLSConfig{
					CertFile: &utils.EnvString{
						Variable: utils.ToPtr("PET_STORE_CERT_FILE"),
					},
					CertPem: &utils.EnvString{
						Variable: utils.ToPtr("PET_STORE_CERT_PEM"),
					},
					KeyFile: &utils.EnvString{
						Variable: utils.ToPtr("PET_STORE_KEY_FILE"),
					},
					KeyPem: &utils.EnvString{
						Variable: utils.ToPtr("PET_STORE_KEY_PEM"),
					},
					CAFile: &utils.EnvString{
						Variable: utils.ToPtr("PET_STORE_CA_FILE"),
					},
					CAPem: &utils.EnvString{
						Variable: utils.ToPtr("PET_STORE_CA_PEM"),
					},
					InsecureSkipVerify: &utils.EnvBool{
						Variable: utils.ToPtr("PET_STORE_INSECURE_SKIP_VERIFY"),
						Value:    utils.ToPtr(true),
					},
					IncludeSystemCACertsPool: &utils.EnvBool{
						Variable: utils.ToPtr("PET_STORE_INCLUDE_SYSTEM_CA_CERT_POOL"),
						Value:    utils.ToPtr(true),
					},
					ServerName: &utils.EnvString{
						Variable: utils.ToPtr("PET_STORE_SERVER_NAME"),
					},
					MinVersion:   "1.0",
					MaxVersion:   "1.3",
					CipherSuites: []string{"TLS_AES_128_GCM_SHA256"},
				},
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
			assert.NilError(t, result.Validate())
			assert.DeepEqual(t, tc.expected.Servers, result.Servers)
			assert.DeepEqual(t, tc.expected.Headers, result.Headers)
			assert.DeepEqual(t, tc.expected.Security, result.Security)

			for key, expectedSS := range tc.expected.SecuritySchemes {
				ss := result.SecuritySchemes[key]
				ss.JSONSchema()
				assert.Equal(t, expectedSS.GetType(), ss.GetType())
				assert.DeepEqual(t, expectedSS.SecuritySchemer, ss.SecuritySchemer, cmp.Exporter(func(t reflect.Type) bool { return true }))
			}

			assert.DeepEqual(t, tc.expected.Version, result.Version)
			assert.DeepEqual(t, *tc.expected.TLS, *result.TLS)
			assert.NilError(t, result.TLS.Validate())

			_, err := json.Marshal(tc.expected)
			if err != nil {
				t.Errorf("failed to encode: %s", err)
				t.FailNow()
			}
		})
	}
}
