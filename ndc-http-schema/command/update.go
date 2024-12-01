package command

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
)

// UpdateCommandArguments represent input arguments of the `update` command
type UpdateCommandArguments struct {
	Dir string `default:"."     env:"HASURA_PLUGIN_CONNECTOR_CONTEXT_PATH"   help:"The directory where the config.yaml file is present" short:"d"`
	Yes bool   `default:"false" help:"Skip the continue confirmation prompt" short:"y"`
}

// UpdateConfiguration updates the configuration for the HTTP connector
func UpdateConfiguration(args *UpdateCommandArguments, logger *slog.Logger) error {
	start := time.Now()
	config, schemas, err := configuration.UpdateHTTPConfiguration(args.Dir, logger)
	if err != nil {
		return err
	}

	validStatus, err := configuration.ValidateConfiguration(config, schemas, logger)
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

	if !args.Yes {
		fmt.Fprint(os.Stderr, "\n\nDeleted configuration warnings. Check your configuration and continue [Y/n]: ")
		var shouldContinue string
		_, err = fmt.Scan(&shouldContinue)
		if err != nil {
			return err
		}

		if !slices.Contains([]string{"y", "yes"}, strings.ToLower(shouldContinue)) {
			return errors.New("Stop the introspection.")
		}
	}

	logger.Info("updated successfully", slog.Duration("exec_time", time.Since(start)))

	return nil
}
