package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hasura/ndc-rest/ndc-rest-schema/openapi"
	"github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/ndc-rest-schema/utils"
	"gopkg.in/yaml.v3"
)

// ConvertCommandArguments represent available command arguments for the convert command
type ConvertCommandArguments struct {
	File                string            `help:"File path needs to be converted."                                                     short:"f"`
	Config              string            `help:"Path of the config file."                                                             short:"c"`
	Output              string            `help:"The location where the ndc schema file will be generated. Print to stdout if not set" short:"o"`
	Spec                string            `help:"The API specification of the file, is one of oas3 (openapi3), oas2 (openapi2)"`
	Format              string            `default:"json"                                                                              help:"The output format, is one of json, yaml. If the output is set, automatically detect the format in the output file extension"`
	Strict              bool              `default:"false"                                                                             help:"Require strict validation"`
	Pure                bool              `default:"false"                                                                             help:"Return the pure NDC schema only"`
	Prefix              string            `help:"Add a prefix to the function and procedure names"`
	TrimPrefix          string            `help:"Trim the prefix in URL, e.g. /v1"`
	EnvPrefix           string            `help:"The environment variable prefix for security values, e.g. PET_STORE"`
	MethodAlias         map[string]string `help:"Alias names for HTTP method. Used for prefix renaming, e.g. getUsers, postUser"`
	AllowedContentTypes []string          `help:"Allowed content types. All content types are allowed by default"`
	PatchBefore         []string          `help:"Patch files to be applied into the input file before converting"`
	PatchAfter          []string          `help:"Patch files to be applied into the input file after converting"`
}

// ConvertToNDCSchema converts to NDC REST schema from file
func CommandConvertToNDCSchema(args *ConvertCommandArguments, logger *slog.Logger) error {
	start := time.Now()
	logger.Debug(
		"converting the document to NDC REST schema",
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

	var config ConvertConfig
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

	ResolveConvertConfigArguments(&config, configDir, args)
	result, err := ConvertToNDCSchema(&config, logger)

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

	fmt.Print(string(resultBytes))
	return nil
}

// ConvertConfig represents the content of convert config file
type ConvertConfig struct {
	// File path needs to be converted
	File string `json:"file" jsonschema:"required" yaml:"file"`
	// The API specification of the file, is one of oas3 (openapi3), oas2 (openapi2)
	Spec schema.SchemaSpecType `json:"spec,omitempty" jsonschema:"default=oas3" yaml:"spec"`
	// Alias names for HTTP method. Used for prefix renaming, e.g. getUsers, postUser
	MethodAlias map[string]string `json:"methodAlias,omitempty" yaml:"methodAlias"`
	// Add a prefix to the function and procedure names
	Prefix string `json:"prefix,omitempty" yaml:"prefix"`
	// Trim the prefix in URL, e.g. /v1
	TrimPrefix string `json:"trimPrefix,omitempty" yaml:"trimPrefix"`
	// The environment variable prefix for security values, e.g. PET_STORE
	EnvPrefix string `json:"envPrefix,omitempty" yaml:"envPrefix"`
	// Return the pure NDC schema only
	Pure bool `json:"pure,omitempty" yaml:"pure"`
	// Require strict validation
	Strict bool `json:"strict,omitempty" yaml:"strict"`
	// Patch files to be applied into the input file before converting
	PatchBefore []utils.PatchConfig `json:"patchBefore,omitempty" yaml:"patchBefore"`
	// Patch files to be applied into the input file after converting
	PatchAfter []utils.PatchConfig `json:"patchAfter,omitempty" yaml:"patchAfter"`
	// Allowed content types. All content types are allowed by default
	AllowedContentTypes []string `json:"allowedContentTypes,omitempty" yaml:"allowedContentTypes"`
	// The location where the ndc schema file will be generated. Print to stdout if not set
	Output string `json:"output,omitempty" yaml:"output"`
}

// ConvertToNDCSchema converts to NDC REST schema from config
func ConvertToNDCSchema(config *ConvertConfig, logger *slog.Logger) (*schema.NDCRestSchema, error) {
	rawContent, err := utils.ReadFileFromPath(config.File)
	if err != nil {
		return nil, err
	}

	rawContent, err = utils.ApplyPatch(rawContent, config.PatchBefore)
	if err != nil {
		return nil, err
	}

	var result *schema.NDCRestSchema
	var errs []error
	options := openapi.ConvertOptions{
		MethodAlias:         config.MethodAlias,
		Prefix:              config.Prefix,
		TrimPrefix:          config.TrimPrefix,
		EnvPrefix:           config.EnvPrefix,
		AllowedContentTypes: config.AllowedContentTypes,
		Strict:              config.Strict,
		Logger:              logger,
	}
	switch config.Spec {
	case schema.OpenAPIv3Spec, schema.OAS3Spec:
		result, errs = openapi.OpenAPIv3ToNDCSchema(rawContent, options)
	case schema.OpenAPIv2Spec, (schema.OAS2Spec):
		result, errs = openapi.OpenAPIv2ToNDCSchema(rawContent, options)
	case schema.NDCSpec:
		if err := json.Unmarshal(rawContent, &result); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid spec %s, expected %+v", config.Spec, []schema.SchemaSpecType{schema.OpenAPIv3Spec, schema.OpenAPIv2Spec})
	}

	if result == nil {
		return nil, errors.Join(errs...)
	} else if len(errs) > 0 {
		logger.Error(errors.Join(errs...).Error())
	}

	return utils.ApplyPatchToRestSchema(result, config.PatchAfter)
}

// ResolveConvertConfigArguments resolves convert config arguments
func ResolveConvertConfigArguments(config *ConvertConfig, configDir string, args *ConvertCommandArguments) {
	if args != nil {
		if args.Spec != "" {
			config.Spec = schema.SchemaSpecType(args.Spec)
		}
		if len(args.MethodAlias) > 0 {
			config.MethodAlias = args.MethodAlias
		}
		if args.Prefix != "" {
			config.Prefix = args.Prefix
		}
		if args.TrimPrefix != "" {
			config.TrimPrefix = args.TrimPrefix
		}
		if args.EnvPrefix != "" {
			config.EnvPrefix = args.EnvPrefix
		}
		if args.Pure {
			config.Pure = args.Pure
		}
		if args.Strict {
			config.Strict = args.Strict
		}
		if len(args.AllowedContentTypes) > 0 {
			config.AllowedContentTypes = args.AllowedContentTypes
		}
	}
	if config.Spec == "" {
		config.Spec = schema.OAS3Spec
	}

	if args != nil && args.File != "" {
		config.File = args.File
	} else if config.File != "" {
		config.File = utils.ResolveFilePath(configDir, config.File)
	}

	if args != nil && args.Output != "" {
		config.Output = args.Output
	} else if config.Output != "" {
		config.Output = utils.ResolveFilePath(configDir, config.Output)
	}

	if args != nil && len(args.PatchBefore) > 0 {
		config.PatchBefore = make([]utils.PatchConfig, len(args.PatchBefore))
		for i, p := range args.PatchBefore {
			config.PatchBefore[i] = utils.PatchConfig{
				Path: p,
			}
		}
	} else {
		for i, p := range config.PatchBefore {
			p.Path = utils.ResolveFilePath(configDir, p.Path)
			config.PatchBefore[i] = p
		}
	}

	if args != nil && len(args.PatchAfter) > 0 {
		config.PatchAfter = make([]utils.PatchConfig, len(args.PatchAfter))
		for i, p := range args.PatchAfter {
			config.PatchAfter[i] = utils.PatchConfig{
				Path: p,
			}
		}
	} else {
		for i, p := range config.PatchAfter {
			p.Path = utils.ResolveFilePath(configDir, p.Path)
			config.PatchAfter[i] = p
		}
	}
}
