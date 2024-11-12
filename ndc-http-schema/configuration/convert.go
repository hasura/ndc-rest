package configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hasura/ndc-http/ndc-http-schema/openapi"
	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
)

// ConvertToNDCSchema converts to NDC HTTP schema from config
func ConvertToNDCSchema(config *ConvertConfig, logger *slog.Logger) (*schema.NDCHttpSchema, error) {
	rawContent, err := utils.ReadFileFromPath(config.File)
	if err != nil {
		return nil, err
	}

	rawContent, err = utils.ApplyPatch(rawContent, config.PatchBefore)
	if err != nil {
		return nil, err
	}

	var result *schema.NDCHttpSchema
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
	rawContent = utils.RemoveYAMLSpecialCharacters(rawContent)

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

	return utils.ApplyPatchToHTTPSchema(result, config.PatchAfter)
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
