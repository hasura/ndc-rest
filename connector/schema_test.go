package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
	"gotest.tools/v3/assert"
)

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

	log.Println(string(bodyBytes))
	assert.DeepEqual(t, expectedBody, body)
}
