package configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"gopkg.in/yaml.v3"
)

// UpdateHTTPConfiguration validates and updates the HTTP configuration
func UpdateHTTPConfiguration(configurationDir string, logger *slog.Logger) (*Configuration, []NDCHttpRuntimeSchema, error) {
	config, err := ReadConfigurationFile(configurationDir)
	if err != nil {
		return nil, nil, err
	}

	schemas, errs := BuildSchemaFromConfig(config, configurationDir, logger)
	if len(errs) > 0 {
		printSchemaValidationError(logger, errs)
		if config.Strict {
			return nil, nil, errors.New("failed to build schema files")
		}
	}

	_, validatedSchemas, errs := MergeNDCHttpSchemas(config, schemas)
	if len(errs) > 0 {
		printSchemaValidationError(logger, errs)
		if validatedSchemas == nil || config.Strict {
			return nil, nil, errors.New("invalid http schema")
		}
	}

	// cache the output file to disk
	if config.Output != "" {
		if err := utils.WriteSchemaFile(filepath.Join(configurationDir, config.Output), schemas); err != nil {
			return nil, nil, err
		}
	}

	return config, schemas, nil
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC HTTP schemas", slog.Any("errors", errors))
}

// ReadConfigurationFile reads and decodes the configuration file from the configuration directory
func ReadConfigurationFile(configurationDir string) (*Configuration, error) {
	var config Configuration
	jsonBytes, err := os.ReadFile(configurationDir + "/config.json")
	if err == nil {
		if err = json.Unmarshal(jsonBytes, &config); err != nil {
			return nil, err
		}

		return &config, nil
	}

	if !os.IsNotExist(err) {
		return nil, err
	}

	// try to read and parse yaml file
	yamlBytes, err := os.ReadFile(configurationDir + "/config.yaml")
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		yamlBytes, err = os.ReadFile(configurationDir + "/config.yml")
	}

	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("the config.{json,yaml,yml} file does not exist at %s", configurationDir)
		} else {
			return nil, err
		}
	}

	if err = yaml.Unmarshal(yamlBytes, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
