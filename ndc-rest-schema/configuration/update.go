package configuration

import (
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/hasura/ndc-rest/ndc-rest-schema/utils"
	"gopkg.in/yaml.v3"
)

// UpdateRESTConfiguration validates and updates the REST configuration
func UpdateRESTConfiguration(configurationDir string, logger *slog.Logger) error {
	configFilePath := configurationDir + "/config.yaml"
	rawConfig, err := utils.ReadFileFromPath(configFilePath)
	if err != nil {
		return err
	}
	var config Configuration
	if err := yaml.Unmarshal(rawConfig, &config); err != nil {
		return err
	}

	schemas, errs := BuildSchemaFiles(&config, configurationDir, logger)
	if len(errs) > 0 {
		printSchemaValidationError(logger, errs)
		if config.Strict {
			return errors.New("failed to build schema files")
		}
	}

	validatedResults, _, errs := MergeNDCRestSchemas(schemas)
	if len(errs) > 0 {
		printSchemaValidationError(logger, errs)
		if validatedResults == nil || config.Strict {
			return errors.New("invalid rest schema")
		}
	}

	// cache the output file to disk
	if config.Output == "" {
		return nil
	}

	return utils.WriteSchemaFile(filepath.Join(configurationDir, config.Output), schemas)
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC REST schemas", slog.Any("errors", errors))
}
