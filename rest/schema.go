package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/hasura/ndc-rest-schema/command"
	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

// GetSchema gets the connector's schema.
func (c *RESTConnector) GetSchema(ctx context.Context, configuration *Configuration, _ *State) (schema.SchemaResponseMarshaler, error) {
	return c.rawSchema, nil
}

// build NDC REST schema from file list
func buildSchemaFiles(configDir string, files []command.ConvertConfig, logger *slog.Logger) ([]ndcRestSchemaWithName, map[string][]string) {
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
				name:   file.File,
				schema: schemaOutput,
			}
		}
		if len(errs) > 0 {
			errors[file.File] = errs
		}
	}

	return schemas, errors
}

func buildSchemaFile(configDir string, conf *command.ConvertConfig, logger *slog.Logger) (*rest.NDCRestSchema, error) {
	if conf.File == "" {
		return nil, errors.New("file path is empty")
	}

	if !strings.HasPrefix(conf.File, "/") && !strings.HasPrefix(conf.File, "http") {
		conf.File = path.Join(configDir, conf.File)
	}
	ndcSchema, err := command.ConvertToNDCSchema(conf, logger)
	if err != nil {
		return nil, err
	}

	defaultArguments := buildRESTArguments(ndcSchema)
	if len(defaultArguments) == 0 {
		return ndcSchema, nil
	}

	for _, fn := range ndcSchema.Functions {
		for key, arg := range defaultArguments {
			fn.Arguments[key] = arg
		}
	}

	for _, proc := range ndcSchema.Procedures {
		for key, arg := range defaultArguments {
			proc.Arguments[key] = arg
		}
	}

	return ndcSchema, nil
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

func buildRESTArguments(restSchema *rest.NDCRestSchema) map[string]schema.ArgumentInfo {
	if restSchema.Settings == nil || len(restSchema.Settings.Servers) < 2 {
		return nil
	}

	var serverIDs []string
	for i, server := range restSchema.Settings.Servers {
		if server.ID != "" {
			serverIDs = append(serverIDs, server.ID)
		} else {
			serverIDs = append(serverIDs, fmt.Sprint(i))
		}
	}
	serverScalar := schema.NewScalarType()
	serverScalar.Representation = schema.NewTypeRepresentationEnum(serverIDs).Encode()
	strategyScalar := RESTExecutionStrategy("")

	restSchema.ScalarTypes[RESTServerIDScalarName] = *serverScalar
	restSchema.ScalarTypes[strategyScalar.ScalarName()] = *strategyScalar.ScalarType()

	restOptionsObject := RESTOptions{}
	restOptionsObjectType := restOptionsObject.ObjectType()
	restSchema.ObjectTypes[restOptionsObject.ObjectName()] = *restOptionsObjectType

	return map[string]schema.ArgumentInfo{
		RESTOptionsArgumentName: {
			Description: restOptionsObjectType.Description,
			Type:        schema.NewNullableNamedType(restOptionsObject.ObjectName()).Encode(),
		},
	}
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC REST schemas", slog.Any("errors", errors))
}
