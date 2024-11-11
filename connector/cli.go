package connector

import (
	"github.com/hasura/ndc-sdk-go/connector"
)

// Start and serve the connector API server
func Start[Configuration, State any](HTTPConnector connector.Connector[Configuration, State], options ...connector.ServeOption) error {
	return connector.Start(HTTPConnector, options...)
}
