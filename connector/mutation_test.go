package connector

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hasura/ndc-http/connector/internal/compression"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/ndctest"
	"github.com/hasura/ndc-sdk-go/schema"
	"gotest.tools/v3/assert"
)

func TestRawHTTPRequest(t *testing.T) {
	ndctest.TestConnector(t, NewHTTPConnector(), ndctest.TestConnectorOptions{
		Configuration: "testdata/jsonplaceholder",
		TestDataDir:   "testdata/raw",
	})
}

func TestHTTPConnectorCompression(t *testing.T) {
	postsBody := map[string]any{
		"id":     float64(101),
		"title":  "Hello world",
		"userId": float64(10),
		"body":   "A test post",
	}

	rawPostsBody, err := json.Marshal(postsBody)
	assert.NilError(t, err)

	mux := http.NewServeMux()
	gzipHandleFunc := func(encoding string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				w.Header().Add(rest.ContentTypeHeader, "application/json")
				w.Header().Add(rest.ContentEncodingHeader, encoding)
				w.WriteHeader(http.StatusOK)

				_, err := compression.GzipCompressor{}.Compress(w, rawPostsBody)
				assert.NilError(t, err)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
		}
	}

	mux.HandleFunc("/posts/gzip", gzipHandleFunc(compression.EncodingGzip))
	mux.HandleFunc("/posts/deflate-failed", gzipHandleFunc(compression.EncodingDeflate))

	mux.HandleFunc("/posts/deflate", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Add(rest.ContentTypeHeader, "application/json")
			w.Header().Add(rest.ContentEncodingHeader, compression.EncodingDeflate)
			w.WriteHeader(http.StatusOK)

			_, err := compression.DeflateCompressor{}.Compress(w, rawPostsBody)
			assert.NilError(t, err)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	t.Setenv("SERVER_URL", server.URL)
	connServer, err := connector.NewServer(NewHTTPConnector(), &connector.ServerOptions{
		Configuration: "testdata/compression",
	}, connector.WithoutRecovery())
	assert.NilError(t, err)
	testServer := connServer.BuildTestServer()
	defer testServer.Close()

	t.Run("gzip", func(t *testing.T) {
		rawReqBody, err := json.Marshal(schema.MutationRequest{
			CollectionRelationships: make(schema.MutationRequestCollectionRelationships),
			Operations: []schema.MutationOperation{
				{
					Type:      schema.MutationOperationProcedure,
					Name:      "createPostGzip",
					Arguments: rawPostsBody,
					Fields: schema.NewNestedObject(map[string]schema.FieldEncoder{
						"id":     schema.NewColumnField("id", nil),
						"title":  schema.NewColumnField("title", nil),
						"userId": schema.NewColumnField("userId", nil),
						"body":   schema.NewColumnField("body", nil),
					}).Encode(),
				},
			},
		})

		res, err := http.Post(testServer.URL+"/mutation", "application/json", bytes.NewBuffer(rawReqBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(postsBody).Encode(),
			},
		})
	})

	t.Run("deflate", func(t *testing.T) {
		rawReqBody, err := json.Marshal(schema.MutationRequest{
			CollectionRelationships: make(schema.MutationRequestCollectionRelationships),
			Operations: []schema.MutationOperation{
				{
					Type:      schema.MutationOperationProcedure,
					Name:      "createPostDeflate",
					Arguments: rawPostsBody,
					Fields: schema.NewNestedObject(map[string]schema.FieldEncoder{
						"id":     schema.NewColumnField("id", nil),
						"title":  schema.NewColumnField("title", nil),
						"userId": schema.NewColumnField("userId", nil),
						"body":   schema.NewColumnField("body", nil),
					}).Encode(),
				},
			},
		})

		res, err := http.Post(testServer.URL+"/mutation", "application/json", bytes.NewBuffer(rawReqBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusOK, schema.MutationResponse{
			OperationResults: []schema.MutationOperationResults{
				schema.NewProcedureResult(postsBody).Encode(),
			},
		})
	})

	t.Run("deflate_failure", func(t *testing.T) {
		rawReqBody, err := json.Marshal(schema.MutationRequest{
			CollectionRelationships: make(schema.MutationRequestCollectionRelationships),
			Operations: []schema.MutationOperation{
				{
					Type:      schema.MutationOperationProcedure,
					Name:      "createPostDeflateFailed",
					Arguments: rawPostsBody,
					Fields: schema.NewNestedObject(map[string]schema.FieldEncoder{
						"id":     schema.NewColumnField("id", nil),
						"title":  schema.NewColumnField("title", nil),
						"userId": schema.NewColumnField("userId", nil),
						"body":   schema.NewColumnField("body", nil),
					}).Encode(),
				},
			},
		})

		res, err := http.Post(testServer.URL+"/mutation", "application/json", bytes.NewBuffer(rawReqBody))
		assert.NilError(t, err)
		assertHTTPResponse(t, res, http.StatusInternalServerError, schema.ErrorResponse{
			Message: "zlib: invalid header",
			Details: make(map[string]any),
		})
	})
}
