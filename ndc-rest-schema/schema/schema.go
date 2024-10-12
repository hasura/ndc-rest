package schema

import (
	"encoding/json"

	"github.com/hasura/ndc-sdk-go/schema"
)

// NDCRestSchema extends the [NDC SchemaResponse] with OpenAPI REST information
//
// [NDC schema]: https://github.com/hasura/ndc-sdk-go/blob/1d3339db29e13a170aa8be5ff7fae8394cba0e49/schema/schema.generated.go#L887
type NDCRestSchema struct {
	SchemaRef string           `json:"$schema,omitempty"  mapstructure:"$schema"  yaml:"$schema,omitempty"`
	Settings  *NDCRestSettings `json:"settings,omitempty" mapstructure:"settings" yaml:"settings,omitempty"`

	// Collections which are available for queries
	Collections []schema.CollectionInfo `json:"collections" mapstructure:"collections" yaml:"collections"`

	// Functions (i.e. collections which return a single column and row)
	Functions []*RESTFunctionInfo `json:"functions" mapstructure:"functions" yaml:"functions"`

	// A list of object types which can be used as the types of arguments, or return
	// types of procedures. Names should not overlap with scalar type names.
	ObjectTypes schema.SchemaResponseObjectTypes `json:"object_types" mapstructure:"object_types" yaml:"object_types"`

	// Procedures which are available for execution as part of mutations
	Procedures []*RESTProcedureInfo `json:"procedures" mapstructure:"procedures" yaml:"procedures"`

	// A list of scalar types which will be used as the types of collection columns
	ScalarTypes schema.SchemaResponseScalarTypes `json:"scalar_types" mapstructure:"scalar_types" yaml:"scalar_types"`
}

// NewNDCRestSchema creates a NDCRestSchema instance
func NewNDCRestSchema() *NDCRestSchema {
	return &NDCRestSchema{
		SchemaRef:   "https://raw.githubusercontent.com/hasura/ndc-rest-schema/main/jsonschema/ndc-rest-schema.jsonschema",
		Settings:    &NDCRestSettings{},
		Collections: []schema.CollectionInfo{},
		Functions:   []*RESTFunctionInfo{},
		Procedures:  []*RESTProcedureInfo{},
		ObjectTypes: make(schema.SchemaResponseObjectTypes),
		ScalarTypes: make(schema.SchemaResponseScalarTypes),
	}
}

// ToSchemaResponse converts the instance to NDC schema.SchemaResponse
func (ndc NDCRestSchema) ToSchemaResponse() *schema.SchemaResponse {
	functions := make([]schema.FunctionInfo, len(ndc.Functions))
	for i, fn := range ndc.Functions {
		functions[i] = fn.FunctionInfo
	}
	procedures := make([]schema.ProcedureInfo, len(ndc.Procedures))
	for i, proc := range ndc.Procedures {
		procedures[i] = proc.ProcedureInfo
	}

	return &schema.SchemaResponse{
		Collections: ndc.Collections,
		ObjectTypes: ndc.ObjectTypes,
		ScalarTypes: ndc.ScalarTypes,
		Functions:   functions,
		Procedures:  procedures,
	}
}

type Response struct {
	ContentType string `json:"contentType" mapstructure:"contentType" yaml:"contentType"`
}

// Request represents the HTTP request information of the webhook
type Request struct {
	URL        string               `json:"url,omitempty"        mapstructure:"url"                                              yaml:"url,omitempty"`
	Method     string               `json:"method,omitempty"     jsonschema:"enum=get,enum=post,enum=put,enum=patch,enum=delete" mapstructure:"method"       yaml:"method,omitempty"`
	Type       RequestType          `json:"type,omitempty"       mapstructure:"type"                                             yaml:"type,omitempty"`
	Headers    map[string]EnvString `json:"headers,omitempty"    mapstructure:"headers"                                          yaml:"headers,omitempty"`
	Parameters []RequestParameter   `json:"parameters,omitempty" mapstructure:"parameters"                                       yaml:"parameters,omitempty"`
	Security   AuthSecurities       `json:"security,omitempty"   mapstructure:"security"                                         yaml:"security,omitempty"`
	// configure the request timeout in seconds, default 30s
	Timeout     uint           `json:"timeout,omitempty"     mapstructure:"timeout"     yaml:"timeout,omitempty"`
	Servers     []ServerConfig `json:"servers,omitempty"     mapstructure:"servers"     yaml:"servers,omitempty"`
	RequestBody *RequestBody   `json:"requestBody,omitempty" mapstructure:"requestBody" yaml:"requestBody,omitempty"`
	Response    Response       `json:"response"              mapstructure:"response"    yaml:"response"`
	Retry       *RetryPolicy   `json:"retry,omitempty"       mapstructure:"retry"       yaml:"retry,omitempty"`
}

// Clone copies this instance to a new one
func (r Request) Clone() *Request {
	return &Request{
		URL:         r.URL,
		Method:      r.Method,
		Type:        r.Type,
		Headers:     r.Headers,
		Parameters:  r.Parameters,
		Timeout:     r.Timeout,
		Retry:       r.Retry,
		Security:    r.Security,
		Servers:     r.Servers,
		RequestBody: r.RequestBody,
		Response:    r.Response,
	}
}

// RequestParameter represents an HTTP request parameter
type RequestParameter struct {
	EncodingObject `yaml:",inline"`

	Name         string            `json:"name,omitempty"         mapstructure:"name"                   yaml:"name,omitempty"`
	ArgumentName string            `json:"argumentName,omitempty" mapstructure:"argumentName,omitempty" yaml:"argumentName,omitempty"`
	In           ParameterLocation `json:"in,omitempty"           mapstructure:"in"                     yaml:"in,omitempty"`
	Schema       *TypeSchema       `json:"schema,omitempty"       mapstructure:"schema"                 yaml:"schema,omitempty"`
}

// TypeSchema represents a serializable object of OpenAPI schema
// that is used for validation
type TypeSchema struct {
	Type        string                `json:"type"                 mapstructure:"type"       yaml:"type"`
	Format      string                `json:"format,omitempty"     mapstructure:"format"     yaml:"format,omitempty"`
	Pattern     string                `json:"pattern,omitempty"    mapstructure:"pattern"    yaml:"pattern,omitempty"`
	Nullable    bool                  `json:"nullable,omitempty"   mapstructure:"nullable"   yaml:"nullable,omitempty"`
	Maximum     *float64              `json:"maximum,omitempty"    mapstructure:"maximum"    yaml:"maximum,omitempty"`
	Minimum     *float64              `json:"minimum,omitempty,"   mapstructure:"minimum"    yaml:"minimum,omitempty"`
	MaxLength   *int64                `json:"maxLength,omitempty"  mapstructure:"maxLength"  yaml:"maxLength,omitempty"`
	MinLength   *int64                `json:"minLength,omitempty"  mapstructure:"minLength"  yaml:"minLength,omitempty"`
	Enum        []string              `json:"enum,omitempty"       mapstructure:"enum"       yaml:"enum,omitempty"`
	Items       *TypeSchema           `json:"items,omitempty"      mapstructure:"items"      yaml:"items,omitempty"`
	Properties  map[string]TypeSchema `json:"properties,omitempty" mapstructure:"properties" yaml:"properties,omitempty"`
	Description string                `json:"-"                    yaml:"-"`
	ReadOnly    bool                  `json:"-"                    yaml:"-"`
	WriteOnly   bool                  `json:"-"                    yaml:"-"`
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

// RequestBody defines flexible request body with content types
type RequestBody struct {
	ContentType string                    `json:"contentType,omitempty" mapstructure:"contentType" yaml:"contentType,omitempty"`
	Schema      *TypeSchema               `json:"schema,omitempty"      mapstructure:"schema"      yaml:"schema,omitempty"`
	Encoding    map[string]EncodingObject `json:"encoding,omitempty"    mapstructure:"encoding"    yaml:"encoding,omitempty"`
}

// RESTFunctionInfo extends NDC query function with OpenAPI REST information
type RESTFunctionInfo struct {
	Request             *Request `json:"request" mapstructure:"request" yaml:"request"`
	schema.FunctionInfo `yaml:",inline"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *RESTFunctionInfo) UnmarshalJSON(b []byte) error {
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

	var function schema.FunctionInfo
	if err := function.UnmarshalJSONMap(raw); err != nil {
		return err
	}

	j.FunctionInfo = function
	return nil
}

// RESTProcedureInfo extends NDC mutation procedure with OpenAPI REST information
type RESTProcedureInfo struct {
	Request              *Request `json:"request" mapstructure:"request" yaml:"request"`
	schema.ProcedureInfo `yaml:",inline"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *RESTProcedureInfo) UnmarshalJSON(b []byte) error {
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

	var procedure schema.ProcedureInfo
	if err := procedure.UnmarshalJSONMap(raw); err != nil {
		return err
	}

	j.ProcedureInfo = procedure
	return nil
}

func toPtr[V any](value V) *V {
	return &value
}

func toAnySlice[T any](values []T) []any {
	results := make([]any, len(values))
	for i, v := range values {
		results[i] = v
	}
	return results
}
