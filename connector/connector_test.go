package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
	"gotest.tools/v3/assert"
)

func TestHTTPConnector(t *testing.T) {
	testCases := []struct {
		Name string
		Dir  string
	}{
		{
			Name: "jsonplaceholder",
			Dir:  "testdata/jsonplaceholder",
		},
		{
			Name: "petstore3",
			Dir:  "testdata/petstore3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			server := test_createServer(t, tc.Dir)
			testServer := server.BuildTestServer()

			t.Run("capabilities", func(t *testing.T) {
				filePath := path.Join(tc.Dir, "snapshots/capabilities")
				rawBytes, err := os.ReadFile(filePath)
				if err != nil {
					if !os.IsNotExist(err) {
						t.Errorf("failed to read %s: %s", filePath, err)
						t.FailNow()
					}
					return
				}

				var capabilities schema.CapabilitiesResponse
				assert.NilError(t, json.Unmarshal(rawBytes, &capabilities))
				resp, err := http.Get(fmt.Sprintf("%s/capabilities", testServer.URL))
				assert.NilError(t, err)
				assertHTTPResponse(t, resp, http.StatusOK, capabilities)
			})

			t.Run("schema", func(t *testing.T) {
				filePath := path.Join(tc.Dir, "snapshots/schema")
				rawBytes, err := os.ReadFile(filePath)
				if err != nil {
					if !os.IsNotExist(err) {
						t.Errorf("failed to read %s: %s", filePath, err)
						t.FailNow()
					}
					return
				}

				var expected schema.SchemaResponse
				assert.NilError(t, json.Unmarshal(rawBytes, &expected))
				resp, err := http.Get(fmt.Sprintf("%s/schema", testServer.URL))
				assert.NilError(t, err)
				assertHTTPResponse(t, resp, http.StatusOK, expected)
			})

			assertNdcOperations(t, path.Join(tc.Dir, "query"), fmt.Sprintf("%s/query", testServer.URL))
			assertNdcOperations(t, path.Join(tc.Dir, "mutation"), fmt.Sprintf("%s/mutation", testServer.URL))
		})
	}
}

func TestHTTPConnector_configurationFailure(t *testing.T) {
	c := NewHTTPConnector()
	_, err := c.ParseConfiguration(context.Background(), "")
	assert.ErrorContains(t, err, "the config.{json,yaml,yml} file does not exist at")
}

func TestHTTPConnector_emptyServer(t *testing.T) {
	_, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/server-empty",
	}, connector.WithoutRecovery())
	assert.Error(t, err, "failed to build NDC HTTP schema")
}

func TestHTTPConnector_authentication(t *testing.T) {
	apiKey := "random_api_key"
	bearerToken := "random_bearer_token"
	slog.SetLogLoggerLevel(slog.LevelDebug)
	server := createMockServer(t, apiKey, bearerToken)
	defer server.Close()

	t.Setenv("PET_STORE_URL", server.URL)
	t.Setenv("PET_STORE_API_KEY", apiKey)
	t.Setenv("PET_STORE_BEARER_TOKEN", bearerToken)
	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/auth",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	findPetsBody := []byte(`{
		"collection": "findPets",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {},
		"collection_relationships": {}
	}`)

	t.Run("auth_default_explain", func(t *testing.T) {
		res, err := http.Post(fmt.Sprintf("%s/query/explain", testServer.URL), "application/json", bytes.NewBuffer(findPetsBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{
				"url":     server.URL + "/pet",
				"headers": `{"Api_key":["ran*******(14)"]}`,
			},
		})
	})

	t.Run("auth_default", func(t *testing.T) {
		res, err := http.Post(fmt.Sprintf("%s/query", testServer.URL), "application/json", bytes.NewBuffer(findPetsBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{
					{
						"__value": map[string]any{
							"headers": map[string]any{
								"Content-Type": string("application/json"),
							},
							"response": map[string]any{},
						},
					},
				},
			},
		})
	})

	addPetBody := []byte(`{
		"operations": [
			{
				"type": "procedure",
				"name": "addPet",
				"arguments": {
					"body": {
						"name": "pet"
					}
				}
			}
		],
		"collection_relationships": {}
	}`)

	t.Run("auth_api_key_explain", func(t *testing.T) {
		res, err := http.Post(fmt.Sprintf("%s/mutation/explain", testServer.URL), "application/json", bytes.NewBuffer(addPetBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{
				"url":     server.URL + "/pet",
				"headers": `{"Api_key":["ran*******(14)"],"Content-Type":["application/json"]}`,
				"body":    `{"name":"pet"}`,
			},
		})
	})

	t.Run("auth_api_key", func(t *testing.T) {
		res, err := http.Post(fmt.Sprintf("%s/mutation", testServer.URL), "application/json", bytes.NewBuffer(addPetBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{
					"headers": map[string]any{
						"Content-Type": string("application/json"),
					},
					"response": map[string]any{},
				}).Encode(),
			},
		})
	})

	authBearerBody := []byte(`{
		"collection": "findPetsByStatus",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {
			"headers": {
				"type": "literal",
				"value": {
					"X-Custom-Header": "This is a test"
				}
			},
			"status": {
				"type": "literal",
				"value": "available"
			}
		},
		"collection_relationships": {}
	}`)

	t.Run("auth_bearer_explain", func(t *testing.T) {
		res, err := http.Post(fmt.Sprintf("%s/query/explain", testServer.URL), "application/json", bytes.NewBuffer(authBearerBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.ExplainResponse{
			Details: schema.ExplainResponseDetails{
				"url":     server.URL + "/pet/findByStatus?status=available",
				"headers": `{"Authorization":["Bearer ran*******(19)"],"X-Custom-Header":["This is a test"]}`,
			},
		})
	})

	t.Run("auth_bearer", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			res, err := http.Post(fmt.Sprintf("%s/query", testServer.URL), "application/json", bytes.NewBuffer(authBearerBody))
			assert.NilError(t, err)
			assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
				{
					Rows: []map[string]any{
						{
							"__value": map[string]any{
								"headers":  map[string]any{"Content-Type": string("application/json")},
								"response": map[string]any{},
							},
						},
					},
				},
			})
		}
	})

	t.Run("retry", func(t *testing.T) {
		reqBody := []byte(`{
			"collection": "petRetry",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {},
			"collection_relationships": {}
		}`)

		res, err := http.Post(fmt.Sprintf("%s/query", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
		assert.NilError(t, err)
		assert.Equal(t, http.StatusTooManyRequests, res.StatusCode)
	})

	t.Run("encoding-ndjson", func(t *testing.T) {
		reqBody := []byte(`{
			"operations": [
				{
					"type": "procedure",
					"name": "createModel",
					"arguments": {
						"body": {
							"model": "gpt3.5"
						}
					},
					"fields": {
						"fields": {
							"fields": {
								"completed": {
									"column": "completed",
									"type": "column"
								},
								"status": {
									"column": "status",
									"type": "column"
								}
							},
							"type": "object"
						},
						"type": "array"
					}
				}
			],
			"collection_relationships": {}
		}`)

		res, err := http.Post(fmt.Sprintf("%s/mutation", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{
					"headers": map[string]any{"Content-Type": string("application/x-ndjson")},
					"response": []any{
						map[string]any{"completed": float64(1), "status": string("OK")},
						map[string]any{"completed": float64(0), "status": string("FAILED")},
					},
				}).Encode(),
			},
		})
	})

	// t.Run("encoding-xml", func(t *testing.T) {
	// 	reqBody := []byte(`{
	// 		"operations": [
	// 			{
	// 				"type": "procedure",
	// 				"name": "putPetXml",
	// 				"arguments": {
	// 					"body": {
	// 						"id":   10,
	// 						"name": "doggie",
	// 						"category": {
	// 							"id":   1,
	// 							"name": "Dogs"
	// 						},
	// 						"photoUrls": ["string"],
	// 						"tags": [
	// 							{
	// 								"id":   0,
	// 								"name": "string"
	// 							}
	// 						],
	// 						"status": "available"
	// 					}
	// 				},
	// 				"fields": {
	// 					"fields": {
	// 						"headers": {
	// 							"column": "headers",
	// 							"type": "column"
	// 						},
	// 						"response": {
	// 							"column": "response",
	// 							"fields": {
	// 								"fields": {
	// 									"category": {
	// 										"column": "category",
	// 										"fields": {
	// 											"fields": {
	// 												"id": {
	// 													"column": "id",
	// 													"type": "column"
	// 												},
	// 												"name": {
	// 													"column": "name",
	// 													"type": "column"
	// 												}
	// 											},
	// 											"type": "object"
	// 										},
	// 										"type": "column"
	// 									},
	// 									"field": {
	// 										"column": "field",
	// 										"type": "column"
	// 									},
	// 									"id": {
	// 										"column": "id",
	// 										"type": "column"
	// 									},
	// 									"name": {
	// 										"column": "name",
	// 										"type": "column"
	// 									},
	// 									"photoUrls": {
	// 										"column": "photoUrls",
	// 										"type": "column"
	// 									},
	// 									"status": {
	// 										"column": "status",
	// 										"type": "column"
	// 									},
	// 									"tags": {
	// 										"column": "tags",
	// 										"fields": {
	// 											"fields": {
	// 												"fields": {
	// 													"id": {
	// 														"column": "id",
	// 														"type": "column"
	// 													},
	// 													"name": {
	// 														"column": "name",
	// 														"type": "column"
	// 													}
	// 												},
	// 												"type": "object"
	// 											},
	// 											"type": "array"
	// 										},
	// 										"type": "column"
	// 									}
	// 								},
	// 								"type": "object"
	// 							},
	// 							"type": "column"
	// 						}
	// 					},
	// 					"type": "object"
	// 				}
	// 			}
	// 		],
	// 		"collection_relationships": {}
	// 	}`)

	// 	res, err := http.Post(fmt.Sprintf("%s/mutation", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
	// 	assert.NilError(t, err)
	// 	assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
	// 		OperationResults: []schema.MutationOperationResults{
	// 			schema.NewProcedureResult(map[string]any{
	// 				"headers": map[string]any{"Content-Type": string("application/xml")},
	// 				"response": map[string]any{"headers": nil, "response": map[string]any{
	// 					"id":   int64(10),
	// 					"name": "doggie",
	// 					"category": map[string]any{
	// 						"id":   int64(1),
	// 						"name": "Dogs",
	// 					},
	// 					"photoUrls": []any{"string"},
	// 					"tags": []any{
	// 						map[string]any{
	// 							"id":   int64(0),
	// 							"name": "string",
	// 						},
	// 					},
	// 					"status": "available",
	// 				}},
	// 			}).Encode(),
	// 		},
	// 	})
	// })
}

func TestHTTPConnector_distribution(t *testing.T) {
	apiKey := "random_api_key"
	bearerToken := "random_bearer_token"

	t.Setenv("PET_STORE_API_KEY", apiKey)
	t.Setenv("PET_STORE_BEARER_TOKEN", bearerToken)

	t.Run("distributed_sequence", func(t *testing.T) {
		mock := mockDistributedServer{}
		server := mock.createServer(t)
		defer server.Close()

		t.Setenv("PET_STORE_DOG_URL", fmt.Sprintf("%s/dog", server.URL))
		t.Setenv("PET_STORE_CAT_URL", fmt.Sprintf("%s/cat", server.URL))

		rc := NewHTTPConnector()
		connServer, err := connector.NewServer(rc, &connector.ServerOptions{
			Configuration: "testdata/patch",
		}, connector.WithoutRecovery())
		assert.NilError(t, err)

		testServer := connServer.BuildTestServer()
		defer testServer.Close()

		assert.Equal(t, uint(30), rc.metadata[0].Runtime.Timeout)
		assert.Equal(t, uint(2), rc.metadata[0].Runtime.Retry.Times)
		assert.Equal(t, uint(1000), rc.metadata[0].Runtime.Retry.Delay)
		assert.Equal(t, uint(1000), rc.metadata[0].Runtime.Retry.Delay)
		assert.DeepEqual(t, []int{429, 500}, rc.metadata[0].Runtime.Retry.HTTPStatus)

		reqBody := []byte(`{
			"collection": "findPetsDistributed",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {},
			"collection_relationships": {}
		}`)

		res, err := http.Post(fmt.Sprintf("%s/query", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{

					{"__value": map[string]any{
						"errors": []any{},
						"results": []any{
							map[string]any{
								"data": map[string]any{
									"name": "dog",
								},
								"server": string("dog"),
							},
							map[string]any{
								"data": map[string]any{
									"name": "cat",
								},
								"server": string("cat"),
							},
						},
					}},
				},
			},
		})
		assert.Equal(t, int32(1), mock.catCount)
		assert.Equal(t, int32(1), mock.dogCount)
	})

	t.Run("distributed_parallel", func(t *testing.T) {
		mock := mockDistributedServer{}
		server := mock.createServer(t)
		defer server.Close()

		t.Setenv("PET_STORE_DOG_URL", fmt.Sprintf("%s/dog", server.URL))
		t.Setenv("PET_STORE_CAT_URL", fmt.Sprintf("%s/cat", server.URL))
		rc := NewHTTPConnector()
		connServer, err := connector.NewServer(rc, &connector.ServerOptions{
			Configuration: "testdata/patch",
		}, connector.WithoutRecovery())
		assert.NilError(t, err)

		testServer := connServer.BuildTestServer()
		defer testServer.Close()

		reqBody := []byte(`{
			"operations": [
				{
					"type": "procedure",
					"name": "addPetDistributed",
					"arguments": {
						"body": {
							"name": "pet"
						},
						"httpOptions": {
							"parallel": true
						}
					}
				}
			],
			"collection_relationships": {}
		}`)

		res, err := http.Post(fmt.Sprintf("%s/mutation", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{
					"errors": []any{},
					"results": []any{
						map[string]any{
							"data": map[string]any{
								"name": "dog",
							},
							"server": string("dog"),
						},
						map[string]any{
							"data": map[string]any{
								"name": "cat",
							},
							"server": string("cat"),
						},
					},
				}).Encode(),
			},
		})
		assert.Equal(t, int32(1), mock.catCount)
		assert.Equal(t, int32(1), mock.dogCount)
	})

	t.Run("specify_server", func(t *testing.T) {
		mock := mockDistributedServer{}
		server := mock.createServer(t)
		defer server.Close()

		t.Setenv("PET_STORE_DOG_URL", fmt.Sprintf("%s/dog", server.URL))
		t.Setenv("PET_STORE_CAT_URL", fmt.Sprintf("%s/cat", server.URL))

		rc := NewHTTPConnector()
		connServer, err := connector.NewServer(rc, &connector.ServerOptions{
			Configuration: "testdata/patch",
		}, connector.WithoutRecovery())
		assert.NilError(t, err)

		testServer := connServer.BuildTestServer()
		defer testServer.Close()

		reqBody := []byte(`{
			"collection": "findPetsDistributed",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {
				"httpOptions": {
					"type": "literal",
					"value": {
						"servers": ["cat"]
					}
				}
			},
			"collection_relationships": {}
		}`)

		res, err := http.Post(fmt.Sprintf("%s/query", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{

					{"__value": map[string]any{
						"errors": []any{},
						"results": []any{
							map[string]any{
								"data": map[string]any{
									"name": "cat",
								},
								"server": string("cat"),
							},
						},
					}},
				},
			},
		})
		assert.Equal(t, int32(1), mock.catCount)
		assert.Equal(t, int32(0), mock.dogCount)
	})
}

func TestHTTPConnector_multiSchemas(t *testing.T) {
	mock := mockMultiSchemaServer{}
	server := mock.createServer()
	defer server.Close()

	t.Setenv("CAT_STORE_URL", fmt.Sprintf("%s/cat", server.URL))
	t.Setenv("DOG_STORE_URL", fmt.Sprintf("%s/dog", server.URL))

	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/multi-schemas",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	reqBody := []byte(`{
			"collection": "findCats",
			"query": {
				"fields": {
					"__value": {
						"type": "column",
						"column": "__value"
					}
				}
			},
			"arguments": {},
			"collection_relationships": {}
		}`)

	res, err := http.Post(fmt.Sprintf("%s/query", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
	assert.NilError(t, err)
	assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
		{
			Rows: []map[string]any{
				{"__value": map[string]any{
					"name": "cat",
				}},
			},
		},
	})
	assert.Equal(t, int32(1), mock.catCount)
	assert.Equal(t, int32(0), mock.dogCount)

	reqBody = []byte(`{
		"collection": "findDogs",
		"query": {
			"fields": {
				"__value": {
					"type": "column",
					"column": "__value"
				}
			}
		},
		"arguments": {},
		"collection_relationships": {}
	}`)

	res, err = http.Post(fmt.Sprintf("%s/query", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
	assert.NilError(t, err)

	assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
		{
			Rows: []map[string]any{
				{"__value": map[string]any{
					"name": "dog",
				}},
			},
		},
	})

	assert.Equal(t, int32(1), mock.catCount)
	assert.Equal(t, int32(1), mock.dogCount)
}

func createMockServer(t *testing.T, apiKey string, bearerToken string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	writeResponse := func(w http.ResponseWriter, body string) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
	mux.HandleFunc("/pet", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodPost:
			if r.Header.Get("api_key") != apiKey {
				t.Errorf("invalid api key, expected %s, got %s", apiKey, r.Header.Get("api_key"))
				t.FailNow()
				return
			}
			writeResponse(w, "{}")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/pet/findByStatus", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", bearerToken) {
				t.Fatalf("invalid bearer token, expected %s, got %s", bearerToken, r.Header.Get("Authorization"))
				return
			}
			if r.Header.Get("X-Custom-Header") != "This is a test" {
				t.Fatalf("invalid X-Custom-Header, expected `This is a test`, got %s", r.Header.Get("X-Custom-Header"))
				return
			}

			if r.URL.Query().Encode() != "status=available" {
				t.Fatalf("expected query param: status=available, got: %s", r.URL.Query().Encode())
				return
			}
			writeResponse(w, "{}")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	var requestCount int
	mux.HandleFunc("/pet/retry", func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount > 3 {
			panic("retry count must not be larger than 2")
		}
		w.WriteHeader(http.StatusTooManyRequests)
	})

	mux.HandleFunc("/model", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			user, password, ok := r.BasicAuth()
			if !ok || user != "user" || password != "password" {
				t.Errorf("invalid basic auth, expected user:password, got %s:%s", user, password)
				t.FailNow()
				return
			}

			w.Header().Add("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"completed": 1, "status": "OK"}
{"completed": 0, "status": "FAILED"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/pet/xml", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.Header().Add("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)

			_, _ = w.Write([]byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<pet><category><id>1</id><name>Dogs</name></category><id>10</id><name>doggie</name><photoUrls><photoUrl>string</photoUrl></photoUrls><status>available</status><tags><tag><id>0</id><name>string</name></tag></tags></pet>"))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	return httptest.NewServer(mux)
}

type mockDistributedServer struct {
	dogCount int32
	catCount int32
}

func (mds *mockDistributedServer) createServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	writeResponse := func(w http.ResponseWriter, data []byte) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
	createHandler := func(name string, apiKey string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("api_key") != apiKey {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(fmt.Sprintf(`{"message": "invalid api key, expected %s, got %s"}`, apiKey, r.Header.Get("api_key"))))
				return
			}
			switch r.Method {
			case http.MethodGet:
				writeResponse(w, []byte(fmt.Sprintf(`{"name": "%s"}`, name)))
			case http.MethodPost:
				rawBody, err := io.ReadAll(r.Body)
				assert.NilError(t, err)

				var body struct {
					Name string `json:"name"`
				}
				// log.Printf("request body: %s", string(rawBody))
				err = json.Unmarshal(rawBody, &body)
				assert.NilError(t, err)
				assert.Equal(t, "pet", body.Name)
				writeResponse(w, []byte(fmt.Sprintf(`{"name": "%s"}`, name)))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
		}
	}
	mux.HandleFunc("/cat/pet", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mds.catCount, 1)
		time.Sleep(100 * time.Millisecond)
		createHandler("cat", "cat-secret")(w, r)
	})
	mux.HandleFunc("/dog/pet", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mds.dogCount, 1)
		createHandler("dog", "dog-secret")(w, r)
	})

	return httptest.NewServer(mux)
}

type mockMultiSchemaServer struct {
	dogCount int32
	catCount int32
}

func (mds *mockMultiSchemaServer) createServer() *httptest.Server {
	mux := http.NewServeMux()

	writeResponse := func(w http.ResponseWriter, data []byte) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
	createHandler := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				log.Println("headers", r.Header)
				if r.Header.Get("pet") != name {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				writeResponse(w, []byte(fmt.Sprintf(`{"name": "%s"}`, name)))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
		}
	}
	mux.HandleFunc("/cat/cat", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mds.catCount, 1)
		createHandler("cat")(w, r)
	})
	mux.HandleFunc("/dog/dog", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mds.dogCount, 1)
		createHandler("dog")(w, r)
	})

	return httptest.NewServer(mux)
}

func assertNdcOperations(t *testing.T, dir string, targetURL string) {
	t.Helper()
	queryFiles, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			t.Errorf("failed to read %s: %s", dir, err)
			t.FailNow()
		}
		return
	}
	for _, entry := range queryFiles {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			requestBytes, err := os.ReadFile(path.Join(dir, entry.Name(), "request.json"))
			assert.NilError(t, err)
			expectedBytes, err := os.ReadFile(path.Join(dir, entry.Name(), "expected.json"))
			assert.NilError(t, err)

			var expected any
			assert.NilError(t, json.Unmarshal(expectedBytes, &expected))
			resp, err := http.Post(targetURL, "application/json", bytes.NewBuffer(requestBytes))
			assert.NilError(t, err)
			assertHTTPResponse(t, resp, http.StatusOK, expected)
		})
	}
}

func test_createServer(t *testing.T, dir string) *connector.Server[configuration.Configuration, State] {
	t.Helper()
	c := NewHTTPConnector()
	server, err := connector.NewServer(c, &connector.ServerOptions{
		Configuration: dir,
	}, connector.WithoutRecovery())
	if err != nil {
		t.Errorf("failed to start server: %s", err)
		t.FailNow()
	}

	return server
}

func assertHTTPResponse[B any](t *testing.T, res *http.Response, statusCode int, expectedBody B) {
	t.Helper()
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal("failed to read response body")
	}

	if res.StatusCode != statusCode {
		t.Fatalf("expected status %d, got %d. Body: %s", statusCode, res.StatusCode, string(bodyBytes))
	}

	var body B
	if err = json.Unmarshal(bodyBytes, &body); err != nil {
		t.Errorf("failed to decode json body, got error: %s; body: %s", err, string(bodyBytes))
	}

	assert.DeepEqual(t, expectedBody, body)
}
