package internal

import (
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	restUtils "github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

const (
	ProcedureSendHTTPRequest string          = "sendHttpRequest"
	ScalarRawHTTPMethod      rest.ScalarName = "RawHttpMethod"
	objectTypeRetryPolicy    string          = "RetryPolicy"
)

var httpMethod_enums = []string{"get", "post", "put", "patch", "delete"}

var defaultScalarTypes = map[rest.ScalarName]schema.ScalarType{
	rest.ScalarInt32: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		Representation:      schema.NewTypeRepresentationInt32().Encode(),
	},
	rest.ScalarString: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		Representation:      schema.NewTypeRepresentationString().Encode(),
	},
	rest.ScalarJSON: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		Representation:      schema.NewTypeRepresentationJSON().Encode(),
	},
	ScalarRawHTTPMethod: {
		AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
		ComparisonOperators: map[string]schema.ComparisonOperatorDefinition{},
		Representation:      schema.NewTypeRepresentationEnum(httpMethod_enums).Encode(),
	},
}

// ApplyDefaultConnectorSchema adds default connector schema to the existing schema.
func ApplyDefaultConnectorSchema(input *schema.SchemaResponse, forwardHeaderConfig configuration.ForwardHeadersSettings) (*schema.SchemaResponse, rest.OperationInfo) {
	for _, scalarName := range utils.GetKeys(defaultScalarTypes) {
		if _, ok := input.ScalarTypes[string(scalarName)]; ok {
			continue
		}

		input.ScalarTypes[string(scalarName)] = defaultScalarTypes[scalarName]
	}

	input.ObjectTypes[objectTypeRetryPolicy] = rest.RetryPolicy{}.Schema()
	procSendHttpRequest := schema.ProcedureInfo{
		Name:        ProcedureSendHTTPRequest,
		Description: utils.ToPtr("Send an HTTP request"),
		Arguments: map[string]schema.ArgumentInfo{
			"url": {
				Description: utils.ToPtr("Request URL"),
				Type:        schema.NewNamedType(string(rest.ScalarString)).Encode(),
			},
			"method": {
				Description: utils.ToPtr("Request method"),
				Type:        schema.NewNullableType(schema.NewNamedType(string(ScalarRawHTTPMethod))).Encode(),
			},
			"additionalHeaders": {
				Description: utils.ToPtr("Additional request headers"),
				Type:        schema.NewNullableType(schema.NewNamedType(string(rest.ScalarJSON))).Encode(),
			},
			"body": {
				Description: utils.ToPtr("Request body"),
				Type:        schema.NewNullableType(schema.NewNamedType(string(rest.ScalarJSON))).Encode(),
			},
			"timeout": {
				Description: utils.ToPtr("Request timeout in seconds"),
				Type:        schema.NewNullableType(schema.NewNamedType(string(rest.ScalarInt32))).Encode(),
			},
			"retry": {
				Description: utils.ToPtr("Retry policy"),
				Type:        schema.NewNullableType(schema.NewNamedType(objectTypeRetryPolicy)).Encode(),
			},
		},
		ResultType: schema.NewNullableNamedType(string(rest.ScalarJSON)).Encode(),
	}

	if forwardHeaderConfig.ArgumentField != nil && *forwardHeaderConfig.ArgumentField != "" {
		procSendHttpRequest.Arguments[*forwardHeaderConfig.ArgumentField] = configuration.NewHeadersArgumentInfo().ArgumentInfo
	}

	if forwardHeaderConfig.ResponseHeaders != nil {
		objectTypeName := restUtils.ToPascalCase(procSendHttpRequest.Name) + "HeadersResponse"
		input.ObjectTypes[objectTypeName] = configuration.NewHeaderForwardingResponseObjectType(procSendHttpRequest.ResultType, forwardHeaderConfig.ResponseHeaders).Schema()

		procSendHttpRequest.ResultType = schema.NewNamedType(objectTypeName).Encode()
	}

	input.Procedures = append(input.Procedures, procSendHttpRequest)

	return input, rest.OperationInfo{
		ResultType: procSendHttpRequest.ResultType,
	}
}
