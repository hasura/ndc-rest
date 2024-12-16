package connector

import (
	"testing"

	"github.com/hasura/ndc-sdk-go/ndctest"
)

func TestRawHTTPRequest(t *testing.T) {
	ndctest.TestConnector(t, NewHTTPConnector(), ndctest.TestConnectorOptions{
		Configuration: "testdata/jsonplaceholder",
		TestDataDir:   "testdata/raw",
	})
}
