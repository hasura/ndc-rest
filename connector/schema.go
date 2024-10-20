package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"

	"github.com/hasura/ndc-rest/connector/internal"
	"github.com/hasura/ndc-rest/ndc-rest-schema/command"
	"github.com/hasura/ndc-rest/ndc-rest-schema/configuration"
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	restUtils "github.com/hasura/ndc-rest/ndc-rest-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// GetSchema gets the connector's schema.
func (c *RESTConnector) GetSchema(ctx context.Context, configuration *configuration.Configuration, _ *State) (schema.SchemaResponseMarshaler, error) {
	return c.rawSchema, nil
}

// BuildSchemaFiles build NDC REST schema from file list
func BuildSchemaFiles(configDir string, files []configuration.ConfigItem, logger *slog.Logger) ([]NDCRestSchemaWithName, map[string][]string) {
	schemas := make([]NDCRestSchemaWithName, len(files))
	errors := make(map[string][]string)
	for i, file := range files {
		var errs []string
		schemaOutput, err := buildSchemaFile(configDir, &file, logger)
		if err != nil {
			errs = append(errs, err.Error())
		}
		if schemaOutput != nil {
			schemas[i] = NDCRestSchemaWithName{
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

func buildSchemaFile(configDir string, conf *configuration.ConfigItem, logger *slog.Logger) (*rest.NDCRestSchema, error) {
	if conf.ConvertConfig.File == "" {
		return nil, errFilePathRequired
	}
	command.ResolveConvertConfigArguments(&conf.ConvertConfig, configDir, nil)
	ndcSchema, err := command.ConvertToNDCSchema(&conf.ConvertConfig, logger)
	if err != nil {
		return nil, err
	}

	if ndcSchema.Settings == nil || len(ndcSchema.Settings.Servers) == 0 {
		return nil, fmt.Errorf("the servers setting of schema %s is empty", conf.ConvertConfig.File)
	}

	buildRESTArguments(ndcSchema, conf)

	return ndcSchema, nil
}

// ApplyNDCRestSchemas applies slice of raw NDC REST schemas to the connector
func (c *RESTConnector) ApplyNDCRestSchemas(schemas []NDCRestSchemaWithName) map[string][]string {
	ndcSchema := &rest.NDCRestSchema{
		ScalarTypes: make(schema.SchemaResponseScalarTypes),
		ObjectTypes: make(map[string]rest.ObjectType),
		Functions:   make(map[string]rest.OperationInfo),
		Procedures:  make(map[string]rest.OperationInfo),
	}
	errors := make(map[string][]string)

	for _, item := range schemas {
		settings := item.schema.Settings
		if settings == nil {
			settings = &rest.NDCRestSettings{}
		} else {
			for i, server := range settings.Servers {
				if server.Retry == nil {
					if settings.Retry != nil {
						server.Retry = settings.Retry
					}
				} else if settings.Retry != nil {
					delay, err := server.Retry.Delay.Value()
					if err != nil {
						errors[fmt.Sprintf("settings.servers[%d].retry.delay", i)] = []string{err.Error()}
						return errors
					}
					if delay == nil || *delay <= 0 {
						server.Retry.Delay = settings.Retry.Delay
					}

					times, err := server.Retry.Times.Value()
					if err != nil {
						errors[fmt.Sprintf("settings.servers[%d].retry.times", i)] = []string{err.Error()}
						return errors
					}
					if times == nil || *times <= 0 {
						server.Retry.Times = settings.Retry.Times
					}

					status, err := server.Retry.HTTPStatus.Value()
					if err != nil {
						errors[fmt.Sprintf("settings.servers[%d].retry.httpStatus", i)] = []string{err.Error()}
						return errors
					}
					if len(status) == 0 {
						server.Retry.HTTPStatus = settings.Retry.HTTPStatus
					}
				}

				if server.Timeout == nil {
					server.Timeout = settings.Timeout
				} else {
					timeout, err := server.Timeout.Value()
					if err != nil {
						errors[fmt.Sprintf("settings.servers[%d].timeout", i)] = []string{err.Error()}
						return errors
					}
					if timeout == nil || *timeout <= 0 {
						settings.Timeout = rest.NewEnvIntValue(*timeout)
					}
				}

				if server.Security.IsEmpty() {
					server.Security = settings.Security
				}
				if server.SecuritySchemes == nil {
					server.SecuritySchemes = make(map[string]rest.SecurityScheme)
				}
				for key, scheme := range settings.SecuritySchemes {
					_, ok := server.SecuritySchemes[key]
					if !ok {
						server.SecuritySchemes[key] = scheme
					}
				}

				if server.Headers == nil {
					server.Headers = make(map[string]rest.EnvString)
				}
				for key, value := range settings.Headers {
					_, ok := server.Headers[key]
					if !ok {
						server.Headers[key] = value
					}
				}
				settings.Servers[i] = server
			}
		}
		meta := rest.NDCRestSchema{
			Settings:   settings,
			Functions:  map[string]rest.OperationInfo{},
			Procedures: map[string]rest.OperationInfo{},
		}
		var errs []string

		for name, scalar := range item.schema.ScalarTypes {
			if originScalar, ok := ndcSchema.ScalarTypes[name]; !ok {
				ndcSchema.ScalarTypes[name] = scalar
			} else if !rest.IsDefaultScalar(name) && !reflect.DeepEqual(originScalar, scalar) {
				slog.Warn(fmt.Sprintf("Scalar type %s is conflicted", name))
			}
		}
		for name, object := range item.schema.ObjectTypes {
			if _, ok := ndcSchema.ObjectTypes[name]; !ok {
				ndcSchema.ObjectTypes[name] = object
			} else {
				slog.Warn(fmt.Sprintf("Object type %s is conflicted", name))
			}
		}

		for fnName, fnItem := range item.schema.Functions {
			if fnItem.Request == nil || fnItem.Request.URL == "" {
				continue
			}
			req, err := validateRequestSchema(fnItem.Request, "get")
			if err != nil {
				errs = append(errs, fmt.Sprintf("function %s: %s", fnName, err))
				continue
			}
			fn := rest.OperationInfo{
				Request:     req,
				Arguments:   fnItem.Arguments,
				Description: fnItem.Description,
				ResultType:  fnItem.ResultType,
			}
			meta.Functions[fnName] = fn
			ndcSchema.Functions[fnName] = fn
		}

		for procName, procItem := range item.schema.Procedures {
			if procItem.Request == nil || procItem.Request.URL == "" {
				continue
			}
			req, err := validateRequestSchema(procItem.Request, "")
			if err != nil {
				errs = append(errs, fmt.Sprintf("procedure %s: %s", procName, err))
				continue
			}

			proc := rest.OperationInfo{
				Request:     req,
				Arguments:   procItem.Arguments,
				Description: procItem.Description,
				ResultType:  procItem.ResultType,
			}
			meta.Procedures[procName] = proc
			ndcSchema.Procedures[procName] = proc
		}

		if len(errs) > 0 {
			errors[item.name] = errs
			continue
		}
		c.metadata = append(c.metadata, meta)
	}

	schemaBytes, err := json.Marshal(ndcSchema.ToSchemaResponse())
	if err != nil {
		errors["schema"] = []string{err.Error()}
	}

	if len(errors) > 0 {
		return errors
	}

	c.schema = &rest.NDCRestSchema{
		ScalarTypes: ndcSchema.ScalarTypes,
		ObjectTypes: ndcSchema.ObjectTypes,
	}
	c.rawSchema = schema.NewRawSchemaResponseUnsafe(schemaBytes)
	return nil
}

func validateRequestSchema(req *rest.Request, defaultMethod string) (*rest.Request, error) {
	if req.Method == "" {
		if defaultMethod == "" {
			return nil, errHTTPMethodRequired
		}
		req.Method = defaultMethod
	}

	if req.Type == "" {
		req.Type = rest.RequestTypeREST
	}

	return req, nil
}

func buildRESTArguments(restSchema *rest.NDCRestSchema, conf *configuration.ConfigItem) {
	if restSchema.Settings == nil || len(restSchema.Settings.Servers) < 2 {
		return
	}

	var serverIDs []string
	for i, server := range restSchema.Settings.Servers {
		if server.ID != "" {
			serverIDs = append(serverIDs, server.ID)
		} else {
			server.ID = strconv.Itoa(i)
			restSchema.Settings.Servers[i] = server
			serverIDs = append(serverIDs, server.ID)
		}
	}

	serverScalar := schema.NewScalarType()
	serverScalar.Representation = schema.NewTypeRepresentationEnum(serverIDs).Encode()

	restSchema.ScalarTypes[internal.RESTServerIDScalarName] = *serverScalar
	restSchema.ObjectTypes[internal.RESTSingleOptionsObjectName] = internal.SingleObjectType

	restSingleOptionsArgument := rest.ArgumentInfo{
		ArgumentInfo: schema.ArgumentInfo{
			Description: internal.SingleObjectType.Description,
			Type:        schema.NewNullableNamedType(internal.RESTSingleOptionsObjectName).Encode(),
		},
	}

	for _, fn := range restSchema.Functions {
		fn.Arguments[internal.RESTOptionsArgumentName] = restSingleOptionsArgument
	}

	for _, proc := range restSchema.Procedures {
		proc.Arguments[internal.RESTOptionsArgumentName] = restSingleOptionsArgument
	}

	if !conf.Distributed {
		return
	}

	restSchema.ObjectTypes[internal.RESTDistributedOptionsObjectName] = internal.DistributedObjectType
	restSchema.ObjectTypes[internal.DistributedErrorObjectName] = rest.ObjectType{
		Description: utils.ToPtr("The error response of the remote request"),
		Fields: map[string]rest.ObjectField{
			"server": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Identity of the remote server"),
					Type:        schema.NewNamedType(internal.RESTServerIDScalarName).Encode(),
				},
			},
			"message": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("An optional human-readable summary of the error"),
					Type:        schema.NewNullableType(schema.NewNamedType(string(rest.ScalarString))).Encode(),
				},
			},
			"details": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Any additional structured information about the error"),
					Type:        schema.NewNullableType(schema.NewNamedType(string(rest.ScalarJSON))).Encode(),
				},
			},
		},
	}

	functionKeys := utils.GetKeys(restSchema.Functions)
	for _, key := range functionKeys {
		fn := restSchema.Functions[key]
		funcName := buildDistributedName(key)
		distributedFn := rest.OperationInfo{
			Request:     fn.Request,
			Arguments:   cloneDistributedArguments(fn.Arguments),
			Description: fn.Description,
			ResultType:  schema.NewNamedType(buildDistributedResultObjectType(restSchema, funcName, fn.ResultType)).Encode(),
		}
		restSchema.Functions[funcName] = distributedFn
	}

	procedureKeys := utils.GetKeys(restSchema.Procedures)
	for _, key := range procedureKeys {
		proc := restSchema.Procedures[key]
		procName := buildDistributedName(key)

		distributedProc := rest.OperationInfo{
			Request:     proc.Request,
			Arguments:   cloneDistributedArguments(proc.Arguments),
			Description: proc.Description,
			ResultType:  schema.NewNamedType(buildDistributedResultObjectType(restSchema, procName, proc.ResultType)).Encode(),
		}
		restSchema.Procedures[procName] = distributedProc
	}
}

func cloneDistributedArguments(arguments map[string]rest.ArgumentInfo) map[string]rest.ArgumentInfo {
	result := map[string]rest.ArgumentInfo{}
	for k, v := range arguments {
		if k != internal.RESTOptionsArgumentName {
			result[k] = v
		}
	}
	result[internal.RESTOptionsArgumentName] = rest.ArgumentInfo{
		ArgumentInfo: schema.ArgumentInfo{
			Description: internal.DistributedObjectType.Description,
			Type:        schema.NewNullableNamedType(internal.RESTDistributedOptionsObjectName).Encode(),
		},
	}
	return result
}

func buildDistributedResultObjectType(restSchema *rest.NDCRestSchema, operationName string, underlyingType schema.Type) string {
	distResultType := restUtils.StringSliceToPascalCase([]string{operationName, "Result"})
	distResultDataType := distResultType + "Data"

	restSchema.ObjectTypes[distResultDataType] = rest.ObjectType{
		Description: utils.ToPtr("Distributed response data of " + operationName),
		Fields: map[string]rest.ObjectField{
			"server": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Identity of the remote server"),
					Type:        schema.NewNamedType(internal.RESTServerIDScalarName).Encode(),
				},
			},
			"data": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("A result of " + operationName),
					Type:        underlyingType,
				},
			},
		},
	}

	restSchema.ObjectTypes[distResultType] = rest.ObjectType{
		Description: utils.ToPtr("Distributed responses of " + operationName),
		Fields: map[string]rest.ObjectField{
			"results": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Results of " + operationName),
					Type:        schema.NewArrayType(schema.NewNamedType(distResultDataType)).Encode(),
				},
			},
			"errors": {
				ObjectField: schema.ObjectField{
					Description: utils.ToPtr("Error responses of " + operationName),
					Type:        schema.NewArrayType(schema.NewNamedType(internal.DistributedErrorObjectName)).Encode(),
				},
			},
		},
	}

	return distResultType
}

func buildDistributedName(name string) string {
	return name + "Distributed"
}

func printSchemaValidationError(logger *slog.Logger, errors map[string][]string) {
	logger.Error("errors happen when validating NDC REST schemas", slog.Any("errors", errors))
}

func parseRESTOptionsFromArguments(arguments map[string]rest.ArgumentInfo, rawRestOptions any) (*internal.RESTOptions, error) {
	var result internal.RESTOptions
	if err := result.FromValue(rawRestOptions); err != nil {
		return nil, err
	}
	argInfo, ok := arguments[internal.RESTOptionsArgumentName]
	if !ok {
		return &result, nil
	}
	restOptionsNamedType := schema.GetUnderlyingNamedType(argInfo.Type)
	result.Distributed = restOptionsNamedType != nil && restOptionsNamedType.Name == internal.RESTDistributedOptionsObjectName
	return &result, nil
}
