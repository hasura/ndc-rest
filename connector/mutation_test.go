package connector

import (
	"log/slog"
	"testing"

	"github.com/hasura/ndc-sdk-go/ndctest"
)

func TestRawHTTPRequest(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	ndctest.TestConnector(t, NewHTTPConnector(), ndctest.TestConnectorOptions{
		Configuration: "testdata/jsonplaceholder",
		TestDataDir:   "testdata/raw",
	})
}
