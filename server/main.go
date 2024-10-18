package main

import (
	rest "github.com/hasura/ndc-rest/connector"
	"github.com/hasura/ndc-sdk-go/connector"
)

// Start the connector server at http://localhost:8080
//
//	go run . serve
//
// See [NDC Go SDK] for more information.
//
// [NDC Go SDK]: https://github.com/hasura/ndc-sdk-go
func main() {
	if err := rest.Start[rest.Configuration, rest.State](
		rest.NewRESTConnector(),
		connector.WithMetricsPrefix("ndc_rest"),
		connector.WithDefaultServiceName("ndc_rest"),
	); err != nil {
		panic(err)
	}
}
