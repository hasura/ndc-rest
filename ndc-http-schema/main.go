package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/hasura/ndc-http/ndc-http-schema/command"
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"github.com/hasura/ndc-http/ndc-http-schema/version"
	"github.com/lmittmann/tint"
)

var cli struct {
	LogLevel  string                                `default:"info" enum:"debug,info,warn,error"                                                                                    help:"Log level."`
	Update    command.UpdateCommandArguments        `cmd:""         help:"Update HTTP connector configuration"`
	Convert   configuration.ConvertCommandArguments `cmd:""         help:"Convert API spec to NDC schema. For example:\n ndc-http-schema convert -f petstore.yaml -o petstore.json"`
	Json2Yaml command.Json2YamlCommandArguments     `cmd:""         help:"Convert JSON file to YAML. For example:\n ndc-http-schema json2yaml -f petstore.json -o petstore.yaml"    name:"json2yaml"`
	Version   struct{}                              `cmd:""         help:"Print the CLI version."`
}

func main() {
	cmd := kong.Parse(&cli, kong.UsageOnError())
	logger, err := initLogger(cli.LogLevel)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
		return
	}

	switch cmd.Command() {
	case "update":
		err = command.UpdateConfiguration(&cli.Update, logger)
	case "convert":
		err = command.CommandConvertToNDCSchema(&cli.Convert, logger)
	case "json2yaml":
		err = command.Json2Yaml(&cli.Json2Yaml, logger)
	case "version":
		_, _ = fmt.Print(version.BuildVersion)
	default:
		logger.Error(fmt.Sprintf("unknown command <%s>", cmd.Command()))
		os.Exit(1)
	}

	if err != nil {
		os.Exit(1)
	}
}

func initLogger(logLevel string) (*slog.Logger, error) {
	var level slog.Level
	err := level.UnmarshalText([]byte(strings.ToUpper(logLevel)))
	if err != nil {
		return nil, err
	}

	logger := slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      level,
		TimeFormat: "15:04",
	}))
	slog.SetDefault(logger)

	return logger, nil
}
