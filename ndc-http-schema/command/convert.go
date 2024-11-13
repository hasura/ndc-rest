package command

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"gopkg.in/yaml.v3"
)

// ConvertToNDCSchema converts to NDC HTTP schema from file
func CommandConvertToNDCSchema(args *configuration.ConvertCommandArguments, logger *slog.Logger) error {
	start := time.Now()
	logger.Debug(
		"converting the document to NDC HTTP schema",
		slog.String("file", args.File),
		slog.String("config", args.Config),
		slog.String("output", args.Output),
		slog.String("spec", args.Spec),
		slog.String("format", args.Format),
		slog.String("prefix", args.Prefix),
		slog.String("trim_prefix", args.TrimPrefix),
		slog.String("env_prefix", args.EnvPrefix),
		slog.Any("patch_before", args.PatchBefore),
		slog.Any("patch_after", args.PatchAfter),
		slog.Any("allowed_content_types", args.AllowedContentTypes),
		slog.Bool("strict", args.Strict),
		slog.Bool("pure", args.Pure),
	)

	if args.File == "" && args.Config == "" {
		err := errors.New("--config or --file argument is required")
		logger.Error(err.Error())
		return err
	}

	configDir, err := os.Getwd()
	if err != nil {
		logger.Error("failed to get work dir: " + err.Error())
		return err
	}

	var config configuration.ConvertConfig
	if args.Config != "" {
		rawConfig, err := utils.ReadFileFromPath(args.Config)
		if err != nil {
			logger.Error(err.Error())
			return err
		}
		if err := yaml.Unmarshal(rawConfig, &config); err != nil {
			logger.Error(err.Error())
			return err
		}
		configDir = filepath.Dir(args.Config)
	}

	configuration.ResolveConvertConfigArguments(&config, configDir, args)
	result, err := configuration.ConvertToNDCSchema(&config, logger)

	if err != nil {
		logger.Error(err.Error())
		return err
	}

	if config.Output != "" {
		if config.Pure {
			err = utils.WriteSchemaFile(config.Output, result.ToSchemaResponse())
		} else {
			err = utils.WriteSchemaFile(config.Output, result)
		}
		if err != nil {
			logger.Error("failed to write schema file", slog.String("error", err.Error()))
			return err
		}

		logger.Info("generated successfully", slog.Duration("execution_time", time.Since(start)))
		return nil
	}

	// print to stderr
	format := schema.SchemaFileJSON
	if args.Format != "" {
		format, err = schema.ParseSchemaFileFormat(args.Format)
		if err != nil {
			logger.Error("failed to parse format", slog.Any("error", err))

			return err
		}
	}

	var rawResult any = result
	if config.Pure {
		rawResult = result.ToSchemaResponse()
	}

	resultBytes, err := utils.MarshalSchema(rawResult, format)
	if err != nil {
		logger.Error("failed to encode schema", slog.Any("error", err))

		return err
	}

	fmt.Fprint(os.Stdout, string(resultBytes))

	return nil
}
