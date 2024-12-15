package schema

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"

	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// NDCHttpSchema extends the [NDC SchemaResponse] with OpenAPI HTTP information
//
// [NDC schema]: https://github.com/hasura/ndc-sdk-go/blob/1d3339db29e13a170aa8be5ff7fae8394cba0e49/schema/schema.generated.go#L887
type NDCHttpSchema struct {
	SchemaRef string           `json:"$schema,omitempty"  mapstructure:"$schema"  yaml:"$schema,omitempty"`
	Settings  *NDCHttpSettings `json:"settings,omitempty" mapstructure:"settings" yaml:"settings,omitempty"`

	// Functions (i.e. collections which return a single column and row)
	Functions map[string]OperationInfo `json:"functions" mapstructure:"functions" yaml:"functions"`

	// A list of object types which can be used as the types of arguments, or return
	// types of procedures. Names should not overlap with scalar type names.
	ObjectTypes map[string]ObjectType `json:"object_types" mapstructure:"object_types" yaml:"object_types"`

	// Procedures which are available for execution as part of mutations
	Procedures map[string]OperationInfo `json:"procedures" mapstructure:"procedures" yaml:"procedures"`

	// A list of scalar types which will be used as the types of collection columns
	ScalarTypes schema.SchemaResponseScalarTypes `json:"scalar_types" mapstructure:"scalar_types" yaml:"scalar_types"`
}

// NewNDCHttpSchema creates a NDCHttpSchema instance
func NewNDCHttpSchema() *NDCHttpSchema {
	return &NDCHttpSchema{
		SchemaRef:   "https://raw.githubusercontent.com/hasura/ndc-http/refs/heads/main/ndc-http-schema/jsonschema/ndc-http-schema.schema.json",
		Settings:    &NDCHttpSettings{},
		Functions:   map[string]OperationInfo{},
		Procedures:  map[string]OperationInfo{},
		ObjectTypes: make(map[string]ObjectType),
		ScalarTypes: make(schema.SchemaResponseScalarTypes),
	}
}

// ToSchemaResponse converts the instance to NDC schema.SchemaResponse
func (ndc NDCHttpSchema) ToSchemaResponse() *schema.SchemaResponse {
	functionKeys := utils.GetSortedKeys(ndc.Functions)
	functions := make([]schema.FunctionInfo, len(functionKeys))
	for i, key := range functionKeys {
		fn := ndc.Functions[key]
		functions[i] = fn.FunctionSchema(key)
	}

	procedureKeys := utils.GetSortedKeys(ndc.Procedures)
	procedures := make([]schema.ProcedureInfo, len(procedureKeys))
	for i, key := range procedureKeys {
		proc := ndc.Procedures[key]
		procedures[i] = proc.ProcedureSchema(key)
	}
	objectTypes := make(schema.SchemaResponseObjectTypes)
	for key, object := range ndc.ObjectTypes {
		objectTypes[key] = object.Schema()
	}

	return &schema.SchemaResponse{
		Collections: []schema.CollectionInfo{},
		ScalarTypes: ndc.ScalarTypes,
		ObjectTypes: objectTypes,
		Functions:   functions,
		Procedures:  procedures,
	}
}

// GetFunction gets the NDC function by name
func (rm NDCHttpSchema) GetFunction(name string) *OperationInfo {
	fn, ok := rm.Functions[name]
	if !ok {
		return nil
	}

	return &fn
}

// GetProcedure gets the NDC procedure by name
func (rm NDCHttpSchema) GetProcedure(name string) *OperationInfo {
	fn, ok := rm.Procedures[name]
	if !ok {
		return nil
	}

	return &fn
}

// AddScalar adds a new scalar if not exist.
func (rm *NDCHttpSchema) AddScalar(name string, scalar schema.ScalarType) {
	_, ok := rm.ScalarTypes[name]
	if !ok {
		rm.ScalarTypes[name] = scalar
	}
}

type Response struct {
	ContentType string `json:"contentType" mapstructure:"contentType" yaml:"contentType"`
}

// RuntimeSettings contain runtime settings for a server
type RuntimeSettings struct { // configure the request timeout in seconds, default 30s
	Timeout uint        `json:"timeout,omitempty" mapstructure:"timeout" yaml:"timeout,omitempty"`
	Retry   RetryPolicy `json:"retry,omitempty"   mapstructure:"retry"   yaml:"retry,omitempty"`
}

// Request represents the HTTP request information of the webhook
type Request struct {
	URL         string                     `json:"url,omitempty"         mapstructure:"url"                                              yaml:"url,omitempty"`
	Method      string                     `json:"method,omitempty"      jsonschema:"enum=get,enum=post,enum=put,enum=patch,enum=delete" mapstructure:"method"        yaml:"method,omitempty"`
	Headers     map[string]utils.EnvString `json:"headers,omitempty"     mapstructure:"headers"                                          yaml:"headers,omitempty"`
	Security    AuthSecurities             `json:"security,omitempty"    mapstructure:"security"                                         yaml:"security,omitempty"`
	Servers     []ServerConfig             `json:"servers,omitempty"     mapstructure:"servers"                                          yaml:"servers,omitempty"`
	RequestBody *RequestBody               `json:"requestBody,omitempty" mapstructure:"requestBody"                                      yaml:"requestBody,omitempty"`
	Response    Response                   `json:"response"              mapstructure:"response"                                         yaml:"response"`

	*RuntimeSettings `yaml:",inline"`
}

// Clone copies this instance to a new one
func (r Request) Clone() *Request {
	return &Request{
		URL:             r.URL,
		Method:          r.Method,
		Headers:         r.Headers,
		Security:        r.Security,
		Servers:         r.Servers,
		RequestBody:     r.RequestBody,
		Response:        r.Response,
		RuntimeSettings: r.RuntimeSettings,
	}
}

// RequestParameter represents an HTTP request parameter
type RequestParameter struct {
	EncodingObject `yaml:",inline"`
	Name           string            `json:"name,omitempty"         mapstructure:"name"                   yaml:"name,omitempty"`
	ArgumentName   string            `json:"argumentName,omitempty" mapstructure:"argumentName,omitempty" yaml:"argumentName,omitempty"`
	In             ParameterLocation `json:"in,omitempty"           mapstructure:"in"                     yaml:"in,omitempty"`
	Schema         *TypeSchema       `json:"schema,omitempty"       mapstructure:"schema"                 yaml:"schema,omitempty"`
}

// TypeSchema represents a serializable object of OpenAPI schema
// that is used for validation
type TypeSchema struct {
	Type        []string    `json:"type"                mapstructure:"type"      yaml:"type"`
	Format      string      `json:"format,omitempty"    mapstructure:"format"    yaml:"format,omitempty"`
	Pattern     string      `json:"pattern,omitempty"   mapstructure:"pattern"   yaml:"pattern,omitempty"`
	Maximum     *float64    `json:"maximum,omitempty"   mapstructure:"maximum"   yaml:"maximum,omitempty"`
	Minimum     *float64    `json:"minimum,omitempty,"  mapstructure:"minimum"   yaml:"minimum,omitempty"`
	MaxLength   *int64      `json:"maxLength,omitempty" mapstructure:"maxLength" yaml:"maxLength,omitempty"`
	MinLength   *int64      `json:"minLength,omitempty" mapstructure:"minLength" yaml:"minLength,omitempty"`
	Items       *TypeSchema `json:"items,omitempty"     mapstructure:"items"     yaml:"items,omitempty"`
	XML         *XMLSchema  `json:"xml,omitempty"       mapstructure:"xml"       yaml:"xml,omitempty"`
	Description string      `json:"-"                   yaml:"-"`
	ReadOnly    bool        `json:"-"                   yaml:"-"`
	WriteOnly   bool        `json:"-"                   yaml:"-"`
}

// RetryPolicy represents the retry policy of request
type RetryPolicy struct {
	// Number of retry times
	Times uint `json:"times,omitempty" mapstructure:"times" yaml:"times,omitempty"`
	// Delay retry delay in milliseconds
	Delay uint `json:"delay,omitempty" mapstructure:"delay" yaml:"delay,omitempty"`
	// HTTPStatus retries if the remote service returns one of these http status
	HTTPStatus []int `json:"httpStatus,omitempty" mapstructure:"httpStatus" yaml:"httpStatus,omitempty"`
}

// Schema returns the object type schema of this type
func (rp RetryPolicy) Schema() schema.ObjectType {
	return schema.ObjectType{
		Description: utils.ToPtr("Retry policy of request"),
		Fields: schema.ObjectTypeFields{
			"times": {
				Description: utils.ToPtr("Number of retry times"),
				Type:        schema.NewNamedType(string(ScalarInt32)).Encode(),
			},
			"delay": {
				Description: utils.ToPtr("Delay retry delay in milliseconds"),
				Type:        schema.NewNullableType(schema.NewNamedType(string(ScalarInt32))).Encode(),
			},
			"httpStatus": {
				Description: utils.ToPtr("List of HTTP status the connector will retry on"),
				Type:        schema.NewNullableType(schema.NewArrayType(schema.NewNamedType(string(ScalarInt32)))).Encode(),
			},
		},
	}
}

// EncodingObject represents the [Encoding Object] that contains serialization strategy for application/x-www-form-urlencoded
//
// [Encoding Object]: https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md#encoding-object
type EncodingObject struct {
	// Describes how a specific property value will be serialized depending on its type.
	// See Parameter Object for details on the style property.
	// The behavior follows the same values as query parameters, including default values.
	// This property SHALL be ignored if the request body media type is not application/x-www-form-urlencoded or multipart/form-data.
	// If a value is explicitly defined, then the value of contentType (implicit or explicit) SHALL be ignored
	Style ParameterEncodingStyle `json:"style,omitempty" mapstructure:"style" yaml:"style,omitempty"`
	// When this is true, property values of type array or object generate separate parameters for each value of the array, or key-value-pair of the map.
	// For other types of properties this property has no effect. When style is form, the default value is true. For all other styles, the default value is false.
	// This property SHALL be ignored if the request body media type is not application/x-www-form-urlencoded or multipart/form-data.
	// If a value is explicitly defined, then the value of contentType (implicit or explicit) SHALL be ignored
	Explode *bool `json:"explode,omitempty" mapstructure:"explode" yaml:"explode,omitempty"`
	// By default, reserved characters :/?#[]@!$&'()*+,;= in form field values within application/x-www-form-urlencoded bodies are percent-encoded when sent.
	// AllowReserved allows these characters to be sent as is:
	AllowReserved bool `json:"allowReserved,omitempty" mapstructure:"allowReserved" yaml:"allowReserved,omitempty"`
	// For more complex scenarios, such as nested arrays or JSON in form data, use the contentType keyword to specify the media type for encoding the value of a complex field.
	ContentType []string `json:"contentType,omitempty" mapstructure:"contentType" yaml:"contentType,omitempty"`
	// A map allowing additional information to be provided as headers, for example Content-Disposition.
	// Content-Type is described separately and SHALL be ignored in this section.
	// This property SHALL be ignored if the request body media type is not a multipart.
	Headers map[string]RequestParameter `json:"headers,omitempty" mapstructure:"headers" yaml:"headers,omitempty"`
}

// SetHeader sets the encoding header
func (eo *EncodingObject) SetHeader(key string, param RequestParameter) {
	if eo.Headers == nil {
		eo.Headers = make(map[string]RequestParameter)
	}
	eo.Headers[key] = param
}

// GetHeader gets the encoding header by key
func (eo *EncodingObject) GetHeader(key string) *RequestParameter {
	if len(eo.Headers) == 0 {
		return nil
	}
	result, ok := eo.Headers[key]
	if !ok {
		return nil
	}

	return &result
}

// RequestBody defines flexible request body with content types
type RequestBody struct {
	ContentType string                    `json:"contentType,omitempty" mapstructure:"contentType" yaml:"contentType,omitempty"`
	Encoding    map[string]EncodingObject `json:"encoding,omitempty"    mapstructure:"encoding"    yaml:"encoding,omitempty"`
}

// OperationInfo extends connector command operation with OpenAPI HTTP information
type OperationInfo struct {
	Request *Request `json:"request" mapstructure:"request" yaml:"request"`
	// Any arguments that this collection requires
	Arguments map[string]ArgumentInfo `json:"arguments" mapstructure:"arguments" yaml:"arguments"`
	// Column description
	Description *string `json:"description,omitempty" mapstructure:"description,omitempty" yaml:"description,omitempty"`
	// The name of the result type
	ResultType schema.Type `json:"result_type" mapstructure:"result_type" yaml:"result_type"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *OperationInfo) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	rawReq, ok := raw["request"]
	if ok {
		var request Request
		if err := json.Unmarshal(rawReq, &request); err != nil {
			return err
		}
		j.Request = &request
	}

	rawArguments, ok := raw["arguments"]
	if ok {
		var arguments map[string]ArgumentInfo
		if err := json.Unmarshal(rawArguments, &arguments); err != nil {
			return err
		}
		j.Arguments = arguments
	}

	rawResultType, ok := raw["result_type"]
	if !ok {
		return errors.New("field result_type in ProcedureInfo: required")
	}
	var resultType schema.Type
	if err := json.Unmarshal(rawResultType, &resultType); err != nil {
		return fmt.Errorf("field result_type in ProcedureInfo: %w", err)
	}
	j.ResultType = resultType

	if rawDescription, ok := raw["description"]; ok {
		var description string
		if err := json.Unmarshal(rawDescription, &description); err != nil {
			return fmt.Errorf("field description in ProcedureInfo: %w", err)
		}
		j.Description = &description
	}

	return nil
}

// Schema returns the connector schema of the function
func (j OperationInfo) FunctionSchema(name string) schema.FunctionInfo {
	arguments := make(schema.FunctionInfoArguments)
	for key, argument := range j.Arguments {
		arguments[key] = argument.ArgumentInfo
	}

	return schema.FunctionInfo{
		Name:        name,
		Arguments:   arguments,
		Description: j.Description,
		ResultType:  j.ResultType,
	}
}

// Schema returns the connector schema of the function
func (j OperationInfo) ProcedureSchema(name string) schema.ProcedureInfo {
	arguments := make(schema.ProcedureInfoArguments)
	for key, argument := range j.Arguments {
		arguments[key] = argument.ArgumentInfo
	}

	return schema.ProcedureInfo{
		Name:        name,
		Arguments:   arguments,
		Description: j.Description,
		ResultType:  j.ResultType,
	}
}

// ObjectType represents the object type of http schema
type ObjectType struct {
	// Description of this type
	Description *string `json:"description,omitempty" mapstructure:"description,omitempty" yaml:"description,omitempty"`
	// Fields defined on this object type
	Fields map[string]ObjectField `json:"fields" mapstructure:"fields" yaml:"fields"`
	// XML schema
	XML *XMLSchema `json:"xml,omitempty" mapstructure:"xml" yaml:"xml,omitempty"`
}

// Schema returns schema the object field
func (of ObjectType) Schema() schema.ObjectType {
	result := schema.ObjectType{
		Description: of.Description,
		Fields:      schema.ObjectTypeFields{},
	}

	for key, field := range of.Fields {
		result.Fields[key] = field.Schema()
	}

	return result
}

// ObjectField defined on this object type
type ObjectField struct {
	schema.ObjectField `yaml:",inline"`

	// The field schema information of the HTTP request
	HTTP *TypeSchema `json:"http,omitempty" mapstructure:"http" yaml:"http,omitempty"`
}

// Schema returns schema the object field
func (of ObjectField) Schema() schema.ObjectField {
	return of.ObjectField
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ObjectField) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	rawType, ok := raw["type"]
	if !ok || len(rawType) == 0 {
		return errors.New("field type in ObjectField: required")
	}

	if err := json.Unmarshal(rawType, &j.Type); err != nil {
		return fmt.Errorf("field type in ObjectField: %w", err)
	}

	if rawDesc, ok := raw["description"]; ok {
		var desc string
		if err := json.Unmarshal(rawDesc, &desc); err != nil {
			return fmt.Errorf("field description in ObjectField: %w", err)
		}
		j.Description = &desc
	}
	if rawArguments, ok := raw["arguments"]; ok {
		var arguments schema.ObjectFieldArguments
		if err := json.Unmarshal(rawArguments, &arguments); err != nil {
			return fmt.Errorf("field arguments in ObjectField: %w", err)
		}
		j.Arguments = arguments
	}

	if rawType, ok := raw["http"]; ok {
		var ty TypeSchema
		if err := json.Unmarshal(rawType, &ty); err != nil {
			return fmt.Errorf("field http in ObjectField: %w", err)
		}
		j.HTTP = &ty
	}

	return nil
}

// ArgumentInfo the information of HTTP request argument
type ArgumentInfo struct {
	schema.ArgumentInfo `yaml:",inline"`

	// The request parameter information of the HTTP request
	HTTP *RequestParameter `json:"http,omitempty" mapstructure:"http" yaml:"http,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ArgumentInfo) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	rawType, ok := raw["type"]
	if !ok || len(rawType) == 0 {
		return errors.New("field type in ArgumentInfo: required")
	}

	if err := json.Unmarshal(rawType, &j.Type); err != nil {
		return fmt.Errorf("field type in ArgumentInfo: %w", err)
	}

	if rawDesc, ok := raw["description"]; ok {
		var desc string
		if err := json.Unmarshal(rawDesc, &desc); err != nil {
			return fmt.Errorf("field description in ArgumentInfo: %w", err)
		}
		j.Description = &desc
	}

	if rawParameter, ok := raw["http"]; ok {
		var param RequestParameter
		if err := json.Unmarshal(rawParameter, &param); err != nil {
			return fmt.Errorf("field http in ArgumentInfo: %w", err)
		}
		j.HTTP = &param
	}

	return nil
}

// XMLSchema represents a XML schema that adds additional metadata to describe the XML representation of this property.
type XMLSchema struct {
	// Replaces the name of the element/attribute used for the described schema property.
	// When defined within items, it will affect the name of the individual XML elements within the list.
	// When defined alongside type being array (outside the items), it will affect the wrapping element and only if wrapped is true.
	// If wrapped is false, it will be ignored.
	Name string `json:"name,omitempty" mapstructure:"name" yaml:"name,omitempty"`
	// The prefix to be used for the name.
	Prefix string `json:"prefix,omitempty" mapstructure:"prefix" yaml:"prefix,omitempty"`
	// The URI of the namespace definition. This MUST be in the form of an absolute URI.
	Namespace string `json:"namespace,omitempty" mapstructure:"namespace" yaml:"namespace,omitempty"`
	// Used only for an array definition. Signifies whether the array is wrapped (for example, <books><book/><book/></books>) or unwrapped (<book/><book/>).
	Wrapped bool `json:"wrapped,omitempty" mapstructure:"wrapped" yaml:"wrapped,omitempty"`
	// Declares whether the property definition translates to an attribute instead of an element.
	Attribute bool `json:"attribute,omitempty" mapstructure:"attribute" yaml:"attribute,omitempty"`
	// Represents a text value of the xml element.
	Text bool `json:"text,omitempty" mapstructure:"text" yaml:"text,omitempty"`
}

// GetFullName gets the full name with prefix.
func (xs XMLSchema) GetFullName() string {
	if xs.Prefix == "" {
		return xs.Name
	}

	return xs.Prefix + ":" + xs.Name
}

// GetNamespaceAttribute gets the namespace attribute
func (xs XMLSchema) GetNamespaceAttribute() xml.Attr {
	// xmlns:smp="http://example.com/schema"
	name := "xmlns"
	if xs.Prefix != "" {
		name += ":" + xs.Prefix
	}

	return xml.Attr{
		Name:  xml.Name{Local: name},
		Value: xs.Namespace,
	}
}

func toAnySlice[T any](values []T) []any {
	results := make([]any, len(values))
	for i, v := range values {
		results[i] = v
	}

	return results
}
