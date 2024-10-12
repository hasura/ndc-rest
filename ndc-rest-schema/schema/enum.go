package schema

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/invopop/jsonschema"
)

// SchemaSpecType represents the spec enum of schema
type SchemaSpecType string

const (
	OpenAPIv3Spec SchemaSpecType = "openapi3"
	OpenAPIv2Spec SchemaSpecType = "openapi2"
	OAS3Spec      SchemaSpecType = "oas3"
	OAS2Spec      SchemaSpecType = "oas2"
	NDCSpec       SchemaSpecType = "ndc"
)

var schemaSpecType_enums = []SchemaSpecType{OAS3Spec, OAS2Spec, OpenAPIv3Spec, OpenAPIv2Spec, NDCSpec}

// JSONSchema is used to generate a custom jsonschema
func (j SchemaSpecType) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: toAnySlice(schemaSpecType_enums),
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *SchemaSpecType) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseSchemaSpecType(rawResult)
	if err != nil {
		return err
	}

	*j = result
	return nil
}

// ParseSchemaSpecType parses SchemaSpecType from string
func ParseSchemaSpecType(value string) (SchemaSpecType, error) {
	result := SchemaSpecType(value)
	if !slices.Contains(schemaSpecType_enums, result) {
		return result, fmt.Errorf("invalid SchemaSpecType. Expected %+v, got <%s>", schemaSpecType_enums, value)
	}
	return result, nil
}

// RequestType represents the request type enum
type RequestType string

const (
	RequestTypeREST RequestType = "rest"
)

var requestType_enums = []RequestType{RequestTypeREST}

// UnmarshalJSON implements json.Unmarshaler.
func (j *RequestType) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseRequestType(rawResult)
	if err != nil {
		return err
	}

	*j = result
	return nil
}

// ParseRequestType parses RequestType from string
func ParseRequestType(value string) (RequestType, error) {
	result := RequestType(value)
	if !slices.Contains(requestType_enums, result) {
		return result, fmt.Errorf("invalid RequestType. Expected %+v, got <%s>", schemaSpecType_enums, value)
	}
	return result, nil
}

// SchemaFileFormat represents the file format enum for NDC REST schema file
type SchemaFileFormat string

const (
	SchemaFileJSON SchemaFileFormat = "json"
	SchemaFileYAML SchemaFileFormat = "yaml"
)

var schemaFileFormat_enums = []SchemaFileFormat{SchemaFileYAML, SchemaFileJSON}

// JSONSchema is used to generate a custom jsonschema
func (j SchemaFileFormat) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: toAnySlice(schemaFileFormat_enums),
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *SchemaFileFormat) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseSchemaFileFormat(rawResult)
	if err != nil {
		return err
	}

	*j = result
	return nil
}

// IsEmpty checks if the style enum is valid
func (j SchemaFileFormat) IsValid() bool {
	return slices.Contains(schemaFileFormat_enums, j)
}

// ParseSchemaFileFormat parses SchemaFileFormat from file extension
func ParseSchemaFileFormat(extension string) (SchemaFileFormat, error) {
	result := SchemaFileFormat(extension)
	if !result.IsValid() {
		return result, fmt.Errorf("invalid SchemaFileFormat. Expected %+v, got <%s>", schemaFileFormat_enums, extension)
	}
	return result, nil
}

// ParameterLocation is [the location] of the parameter.
// Possible values are "query", "header", "path" or "cookie".
//
// [the location]: https://swagger.io/specification/#parameter-object
type ParameterLocation string

const (
	InQuery    ParameterLocation = "query"
	InHeader   ParameterLocation = "header"
	InPath     ParameterLocation = "path"
	InCookie   ParameterLocation = "cookie"
	InBody     ParameterLocation = "body"
	InFormData ParameterLocation = "formData"
)

var parameterLocation_enums = []ParameterLocation{InQuery, InHeader, InPath, InCookie, InBody, InFormData}

// JSONSchema is used to generate a custom jsonschema
func (j ParameterLocation) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: toAnySlice(parameterLocation_enums),
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ParameterLocation) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseParameterLocation(rawResult)
	if err != nil {
		return err
	}

	*j = result
	return nil
}

// IsEmpty checks if the style enum is valid
func (j ParameterLocation) IsValid() bool {
	return slices.Contains(parameterLocation_enums, j)
}

// ParseParameterLocation parses ParameterLocation from string
func ParseParameterLocation(input string) (ParameterLocation, error) {
	result := ParameterLocation(input)
	if !result.IsValid() {
		return result, fmt.Errorf("invalid ParameterLocation. Expected %+v, got <%s>", parameterLocation_enums, input)
	}
	return result, nil
}

// ScalarName defines supported scalar name enums of the OpenAPI spec
type ScalarName string

const (
	ScalarBoolean     ScalarName = "Boolean"
	ScalarString      ScalarName = "String"
	ScalarInt32       ScalarName = "Int32"
	ScalarInt64       ScalarName = "Int64"
	ScalarFloat32     ScalarName = "Float32"
	ScalarFloat64     ScalarName = "Float64"
	ScalarBigDecimal  ScalarName = "BigDecimal"
	ScalarUUID        ScalarName = "UUID"
	ScalarDate        ScalarName = "Date"
	ScalarTimestampTZ ScalarName = "TimestampTZ"
	ScalarBytes       ScalarName = "Bytes"
	ScalarBinary      ScalarName = "Binary"
	ScalarJSON        ScalarName = "JSON"
	ScalarUnixTime    ScalarName = "UnixTime"
	ScalarEmail       ScalarName = "Email"
	ScalarURI         ScalarName = "URI"
	ScalarIPV4        ScalarName = "IPv4"
	ScalarIPV6        ScalarName = "IPv6"
)

var scalarName_enums = []ScalarName{
	ScalarBoolean,
	ScalarString,
	ScalarInt32,
	ScalarInt64,
	ScalarFloat32,
	ScalarFloat64,
	ScalarBigDecimal,
	ScalarUUID,
	ScalarDate,
	ScalarTimestampTZ,
	ScalarBytes,
	ScalarBinary,
	ScalarJSON,
	ScalarUnixTime,
	ScalarEmail,
	ScalarURI,
	ScalarIPV4,
	ScalarIPV6,
}

// IsDefaultScalar checks if the scalar name is
func IsDefaultScalar(name string) bool {
	return slices.Contains(scalarName_enums, ScalarName(name))
}

const (
	ContentTypeHeader            = "Content-Type"
	ContentTypeJSON              = "application/json"
	ContentTypeNdJSON            = "application/x-ndjson"
	ContentTypeXML               = "application/xml"
	ContentTypeFormURLEncoded    = "application/x-www-form-urlencoded"
	ContentTypeMultipartFormData = "multipart/form-data"
	ContentTypeTextPlain         = "text/plain"
	ContentTypeTextHTML          = "text/html"
)

// ParameterEncodingStyle represents the encoding style of the parameter.
// style defines how multiple values are delimited. Possible styles depend on the parameter location – path, query, header or cookie.
type ParameterEncodingStyle string

const (
	// EncodingStyleSimple (default of query) comma-separated values. Corresponds to the {param_name} URI template.
	EncodingStyleSimple ParameterEncodingStyle = "simple"
	// EncodingStyleLabel dot-prefixed values, also known as label expansion. Corresponds to the {.param_name} URI template.
	EncodingStyleLabel ParameterEncodingStyle = "label"
	// EncodingStyleMatrix semicolon-prefixed values, also known as path-style expansion. Corresponds to the {;param_name} URI template.
	EncodingStyleMatrix ParameterEncodingStyle = "matrix"
	// EncodingStyleForm ampersand-separated values, also known as form-style query expansion. Corresponds to the {?param_name} URI template.
	EncodingStyleForm ParameterEncodingStyle = "form"
	// EncodingStyleSpaceDelimited space-separated array values. Same as collectionFormat: ssv in OpenAPI 2.0.
	// Has effect only for non-exploded arrays (explode: false), that is, the space separates the array values if the array is a single parameter, as in arr=a b c.
	EncodingStyleSpaceDelimited ParameterEncodingStyle = "spaceDelimited"
	// EncodingStylePipeDelimited pipeline-separated array values. Same as collectionFormat: pipes in OpenAPI 2.0.
	// Has effect only for non-exploded arrays (explode: false), that is, the pipe separates the array values if the array is a single parameter, as in arr=a|b|c.
	EncodingStylePipeDelimited ParameterEncodingStyle = "pipeDelimited"
	// EncodingStyleDeepObject simple non-nested objects are serialized as paramName[prop1]=value1&paramName[prop2]=value2&....
	// The behavior for nested objects and arrays is undefined.
	EncodingStyleDeepObject ParameterEncodingStyle = "deepObject"
)

var parameterEncodingStyle_enums = []ParameterEncodingStyle{
	EncodingStyleSimple,
	EncodingStyleLabel,
	EncodingStyleMatrix,
	EncodingStyleForm,
	EncodingStyleSpaceDelimited,
	EncodingStylePipeDelimited,
	EncodingStyleDeepObject,
}

// JSONSchema is used to generate a custom jsonschema
func (j ParameterEncodingStyle) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: toAnySlice(parameterEncodingStyle_enums),
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ParameterEncodingStyle) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseParameterEncodingStyle(rawResult)
	if err != nil {
		return err
	}

	*j = result
	return nil
}

// IsEmpty checks if the style enum is valid
func (j ParameterEncodingStyle) IsValid() bool {
	return slices.Contains(parameterEncodingStyle_enums, j)
}

// ParseParameterEncodingStyle parses ParameterEncodingStyle from string
func ParseParameterEncodingStyle(input string) (ParameterEncodingStyle, error) {
	result := ParameterEncodingStyle(input)
	if !result.IsValid() {
		return result, fmt.Errorf("invalid ParameterEncodingStyle. Expected %+v, got <%s>", parameterEncodingStyle_enums, input)
	}
	return result, nil
}
