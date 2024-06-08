package rest

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/hasura/ndc-rest-schema/command"
	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/lmittmann/tint"
)

// CLI extends the NDC SDK with custom commands
type CLI struct {
	connector.ServeCLI
	Convert command.ConvertCommandArguments `cmd:"" help:"Convert API spec to NDC schema. For example:\n ndc-rest-schema convert -f petstore.yaml -o petstore.json"`
}

// Execute executes custom commands
func (c CLI) Execute(ctx context.Context, cmd string) error {
	switch cmd {
	case "convert":
		var logLevel slog.Level
		err := logLevel.UnmarshalText([]byte(strings.ToUpper(c.LogLevel)))
		if err != nil {
			return err
		}
		logger := slog.New(tint.NewHandler(os.Stderr, &tint.Options{
			Level:      logLevel,
			TimeFormat: "15:04",
		}))

		return command.CommandConvertToNDCSchema(&c.Convert, logger)
	default:
		return c.ServeCLI.Execute(ctx, cmd)
	}
}

// Start wrap the connector.Start function with custom CLI
func Start[Configuration, State any](restConnector connector.Connector[Configuration, State], options ...connector.ServeOption) error {
	var cli CLI
	return connector.StartCustom[Configuration, State](&cli, restConnector, options...)
}
