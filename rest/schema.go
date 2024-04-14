package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/hasura/ndc-rest-schema/openapi"
	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"gopkg.in/yaml.v3"
)

// GetSchema gets the connector's schema.
func (c *RESTConnector) GetSchema(ctx context.Context, configuration *Configuration, _ *State) (schema.SchemaResponseMarshaler, error) {
	return c.rawSchema, nil
}

// build NDC REST schema from file list
func buildSchemaFiles(configDir string, files []SchemaFile, logger *slog.Logger) ([]ndcRestSchemaWithName, map[string][]string) {
	schemas := make([]ndcRestSchemaWithName, len(files))
	errors := make(map[string][]string)
	for i, file := range files {
		var errs []string
		schemaOutput, err := buildSchemaFile(configDir, &file, logger)
		if err != nil {
			errs = append(errs, err.Error())
		}
		if schemaOutput != nil {
			schemas[i] = ndcRestSchemaWithName{
				name:   file.Path,
				schema: schemaOutput,
			}
		}
		if len(errs) > 0 {
			errors[file.Path] = errs
		}
	}

	return schemas, errors
}

func buildSchemaFile(configDir string, conf *SchemaFile, logger *slog.Logger) (*rest.NDCRestSchema, error) {
	if conf.Path == "" {
		return nil, errors.New("file path is empty")
	}

	filePath := conf.Path
	if !strings.HasPrefix(conf.Path, "/") && !strings.HasPrefix(conf.Path, "http") {
		filePath = path.Join(configDir, conf.Path)
	}
	rawBytes, err := utils.ReadFileFromPath(filePath)
	if err != nil {
		return nil, err
	}

	switch conf.Spec {
	case rest.NDCSpec:
		var result rest.NDCRestSchema
		fileFormat, err := rest.ParseSchemaFileFormat(strings.Trim(path.Ext(conf.Path), "."))
		if err != nil {
			return nil, err
		}
		switch fileFormat {
		case rest.SchemaFileJSON:
			if err := json.Unmarshal(rawBytes, &result); err != nil {
				return nil, err
			}
		case rest.SchemaFileYAML:
			if err := yaml.Unmarshal(rawBytes, &result); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("invalid file format: %s", fileFormat)
		}

		return &result, nil
	case rest.OpenAPIv2Spec, rest.OAS2Spec:
		result, errs := openapi.OpenAPIv2ToNDCSchema(rawBytes, &openapi.ConvertOptions{
			MethodAlias: conf.MethodAlias,
			TrimPrefix:  conf.TrimPrefix,
		})
		if result != nil {
			if len(errs) > 0 {
				logger.Warn("some errors happened when parsing OpenAPI", slog.Any("errors", errs))
			}
			return result, nil
		}
		return nil, errors.Join(errs...)
	case rest.OpenAPIv3Spec, rest.OAS3Spec:
		result, errs := openapi.OpenAPIv3ToNDCSchema(rawBytes, &openapi.ConvertOptions{
			MethodAlias: conf.MethodAlias,
			TrimPrefix:  conf.TrimPrefix,
		})
		if result != nil {
			if len(errs) > 0 {
				logger.Warn("some errors happened when parsing OpenAPI", slog.Any("errors", errs))
			}
			return result, nil
		}
		return nil, errors.Join(errs...)
	default:
		return nil, fmt.Errorf("invalid configuration spec: %s", conf.Spec)
	}
}

func (c *RESTConnector) applyNDCRestSchemas(schemas []ndcRestSchemaWithName) map[string][]string {
	ndcSchema := &schema.SchemaResponse{
		Collections: []schema.CollectionInfo{},
		ScalarTypes: make(schema.SchemaResponseScalarTypes),
		ObjectTypes: make(schema.SchemaResponseObjectTypes),
	}
	functions := map[string]rest.RESTFunctionInfo{}
	procedures := map[string]rest.RESTProcedureInfo{}
	errors := make(map[string][]string)

	for _, item := range schemas {
		settings := item.schema.Settings
		if settings == nil {
			settings = &rest.NDCRestSettings{}
		}
		meta := RESTMetadata{
			settings: settings,
		}
		var errs []string

		for name, scalar := range item.schema.ScalarTypes {
			ndcSchema.ScalarTypes[name] = scalar
		}
		for name, object := range item.schema.ObjectTypes {
			ndcSchema.ObjectTypes[name] = object
		}
		ndcSchema.Collections = append(ndcSchema.Collections, item.schema.Collections...)

		var functionSchemas []schema.FunctionInfo
		var procedureSchemas []schema.ProcedureInfo
		for _, fnItem := range item.schema.Functions {
			if fnItem.Request == nil || fnItem.Request.URL == "" {
				continue
			}
			req, err := validateRequestSchema(fnItem.Request, "get")
			if err != nil {
				errs = append(errs, fmt.Sprintf("function %s: %s", fnItem.Name, err))
				continue
			}
			fn := rest.RESTFunctionInfo{
				Request:      req,
				FunctionInfo: fnItem.FunctionInfo,
			}
			functions[fnItem.Name] = fn
			functionSchemas = append(functionSchemas, fn.FunctionInfo)
		}

		for _, procItem := range item.schema.Procedures {
			if procItem.Request == nil || procItem.Request.URL == "" {
				continue
			}
			req, err := validateRequestSchema(procItem.Request, "")
			if err != nil {
				errs = append(errs, fmt.Sprintf("procedure %s: %s", procItem.Name, err))
				continue
			}
			procedures[procItem.Name] = rest.RESTProcedureInfo{
				Request:       req,
				ProcedureInfo: procItem.ProcedureInfo,
			}
			procedureSchemas = append(procedureSchemas, procItem.ProcedureInfo)
		}

		if len(errs) > 0 {
			errors[item.name] = errs
			continue
		}
		ndcSchema.Functions = append(ndcSchema.Functions, functionSchemas...)
		ndcSchema.Procedures = append(ndcSchema.Procedures, procedureSchemas...)

		meta.functions = functions
		meta.procedures = procedures
		c.metadata = append(c.metadata, meta)
	}

	schemaBytes, err := json.Marshal(ndcSchema)
	if err != nil {
		errors["schema"] = []string{err.Error()}
	}

	if len(errors) > 0 {
		return errors
	}

	c.schema = &schema.SchemaResponse{
		ScalarTypes: ndcSchema.ScalarTypes,
		ObjectTypes: ndcSchema.ObjectTypes,
	}
	c.rawSchema = schema.NewRawSchemaResponseUnsafe(schemaBytes)
	return nil
}

func validateRequestSchema(req *rest.Request, defaultMethod string) (*rest.Request, error) {
	if req.Method == "" {
		if defaultMethod == "" {
			return nil, fmt.Errorf("the HTTP method is required")
		}
		req.Method = defaultMethod
	}

	if req.Type == "" {
		req.Type = rest.RequestTypeREST
	}

	return req, nil
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC REST schemas", slog.Any("errors", errors))
}
