package command

import (
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
)

// UpdateCommandArguments represent input arguments of the `update` command
type UpdateCommandArguments struct {
	Dir string `default:"." env:"HASURA_PLUGIN_CONNECTOR_CONTEXT_PATH" help:"The directory where the config.yaml file is present" short:"d"`
}

// UpdateConfiguration updates the configuration for the HTTP connector
func UpdateConfiguration(args *UpdateCommandArguments, logger *slog.Logger, noColor bool) error {
	start := time.Now()
	config, schemas, err := configuration.UpdateHTTPConfiguration(args.Dir, logger)
	if err != nil {
		return err
	}

	validStatus, err := configuration.ValidateConfiguration(config, args.Dir, schemas, logger, noColor)
	if err != nil {
		return err
	}

	if validStatus.IsOk() {
		return nil
	}

	validStatus.Render(os.Stderr)
	if validStatus.HasError() {
		return errors.New("Detected configuration errors. Update your configuration and try again.")
	}

	logger.Info("Updated successfully", slog.Duration("exec_time", time.Since(start)))

	return nil
}
