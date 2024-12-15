package main

import (
	rest "github.com/hasura/ndc-http/connector"
	"github.com/hasura/ndc-http/ndc-http-schema/version"
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
	if err := connector.Start(
		rest.NewHTTPConnector(),
		connector.WithMetricsPrefix("ndc_http"),
		connector.WithDefaultServiceName("ndc_http"),
		connector.WithVersion(version.BuildVersion),
	); err != nil {
		panic(err)
	}
}
