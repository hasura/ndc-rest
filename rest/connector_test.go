package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
	"gotest.tools/v3/assert"
)

func TestRESTConnector(t *testing.T) {
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

func TestRESTConnector_configurationFailure(t *testing.T) {
	c := NewRESTConnector()
	_, err := c.ParseConfiguration(context.Background(), "")
	assert.ErrorContains(t, err, "the config.{json,yaml,yml} file does not exist at")
}

func TestRESTConnector_authentication(t *testing.T) {
	apiKey := "random_api_key"
	bearerToken := "random_bearer_token"
	slog.SetLogLoggerLevel(slog.LevelDebug)
	server := createMockServer(t, apiKey, bearerToken)
	defer server.Close()

	t.Setenv("PET_STORE_URL", server.URL)
	t.Setenv("PET_STORE_API_KEY", apiKey)
	t.Setenv("PET_STORE_BEARER_TOKEN", bearerToken)
	connServer, err := connector.NewServer(NewRESTConnector(), &connector.ServerOptions{
		Configuration: "testdata/auth",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	t.Run("auth_default", func(t *testing.T) {
		reqBody := []byte(`{
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

		res, err := http.Post(fmt.Sprintf("%s/query", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.QueryResponse{
			{
				Rows: []map[string]any{
					{"__value": map[string]any{}},
				},
			},
		})
	})

	t.Run("auth_api_key", func(t *testing.T) {
		reqBody := []byte(`{
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

		res, err := http.Post(fmt.Sprintf("%s/mutation", testServer.URL), "application/json", bytes.NewBuffer(reqBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(map[string]any{}).Encode(),
			},
		})
	})

	t.Run("auth_bearer", func(t *testing.T) {
		reqBody := []byte(`{
			"collection": "findPetsByStatus",
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
					{"__value": map[string]any{}},
				},
			},
		})
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
}

func TestRESTConnector_distribution(t *testing.T) {
	apiKey := "random_api_key"
	bearerToken := "random_bearer_token"
	slog.SetLogLoggerLevel(slog.LevelDebug)

	t.Setenv("PET_STORE_API_KEY", apiKey)
	t.Setenv("PET_STORE_BEARER_TOKEN", bearerToken)
	connServer, err := connector.NewServer(NewRESTConnector(), &connector.ServerOptions{
		Configuration: "testdata/patch",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	t.Run("distributed_sequence", func(t *testing.T) {
		mock := mockDistributedServer{}
		server := mock.createServer(t, apiKey)
		defer server.Close()

		t.Setenv("PET_STORE_DOG_URL", fmt.Sprintf("%s/dog", server.URL))
		t.Setenv("PET_STORE_CAT_URL", fmt.Sprintf("%s/cat", server.URL))

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
		server := mock.createServer(t, apiKey)
		defer server.Close()

		t.Setenv("PET_STORE_DOG_URL", fmt.Sprintf("%s/dog", server.URL))
		t.Setenv("PET_STORE_CAT_URL", fmt.Sprintf("%s/cat", server.URL))

		reqBody := []byte(`{
			"operations": [
				{
					"type": "procedure",
					"name": "addPetDistributed",
					"arguments": {
						"body": {
							"name": "pet"
						},
						"restOptions": {
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
		server := mock.createServer(t, apiKey)
		defer server.Close()

		t.Setenv("PET_STORE_DOG_URL", fmt.Sprintf("%s/dog", server.URL))
		t.Setenv("PET_STORE_CAT_URL", fmt.Sprintf("%s/cat", server.URL))

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
				"restOptions": {
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

func createMockServer(t *testing.T, apiKey string, bearerToken string) *httptest.Server {
	mux := http.NewServeMux()

	writeResponse := func(w http.ResponseWriter) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}
	mux.HandleFunc("/pet", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodPost:
			if r.Header.Get("api_key") != apiKey {
				t.Errorf("invalid api key, expected %s, got %s", apiKey, r.Header.Get("api_key"))
				t.FailNow()
				return
			}
			writeResponse(w)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	mux.HandleFunc("/pet/findByStatus", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", bearerToken) {
				t.Errorf("invalid bearer token, expected %s, got %s", bearerToken, r.Header.Get("Authorization"))
				t.FailNow()
				return
			}
			writeResponse(w)
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

	return httptest.NewServer(mux)
}

type mockDistributedServer struct {
	dogCount int32
	catCount int32
}

func (mds *mockDistributedServer) createServer(t *testing.T, apiKey string) *httptest.Server {
	mux := http.NewServeMux()

	writeResponse := func(w http.ResponseWriter, data []byte) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
	createHandler := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("api_key") != apiKey {
				t.Errorf("invalid api key, expected %s, got %s", apiKey, r.Header.Get("api_key"))
				t.FailNow()
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
		createHandler("cat")(w, r)
	})
	mux.HandleFunc("/dog/pet", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mds.dogCount, 1)
		createHandler("dog")(w, r)
	})

	return httptest.NewServer(mux)
}

func assertNdcOperations(t *testing.T, dir string, targetURL string) {
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

func test_createServer(t *testing.T, dir string) *connector.Server[Configuration, State] {
	c := NewRESTConnector()
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
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		t.Error("failed to read response body")
		t.FailNow()
	}

	if res.StatusCode != statusCode {
		t.Errorf("expected status %d, got %d. Body: %s", statusCode, res.StatusCode, string(bodyBytes))
		t.FailNow()
	}

	var body B
	if err = json.Unmarshal(bodyBytes, &body); err != nil {
		t.Errorf("failed to decode json body, got error: %s; body: %s", err, string(bodyBytes))
		t.FailNow()
	}

	assert.DeepEqual(t, expectedBody, body)
}
