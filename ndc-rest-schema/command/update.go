package command

import (
	"log/slog"
	"time"

	"github.com/hasura/ndc-rest/ndc-rest-schema/configuration"
)

// UpdateCommandArguments represent input arguments of the `update` command
type UpdateCommandArguments struct {
	Dir string `help:"The directory where the config.yaml file is present" short:"d" env:"HASURA_PLUGIN_CONNECTOR_CONTEXT_PATH" default:"."`
}

// UpdateConfiguration updates the configuration for the REST connector
func UpdateConfiguration(args *UpdateCommandArguments, logger *slog.Logger) error {
	start := time.Now()
	if err := configuration.UpdateRESTConfiguration(args.Dir, logger); err != nil {
		return err
	}

	logger.Info("updated successfully", slog.Duration("exec_time", time.Since(start)))

	return nil
}
