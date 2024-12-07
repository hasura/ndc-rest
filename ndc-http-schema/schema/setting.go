package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/hasura/ndc-sdk-go/utils"
	"github.com/invopop/jsonschema"
	"github.com/theory/jsonpath"
	"github.com/theory/jsonpath/spec"
	orderedmap "github.com/wk8/go-ordered-map/v2"
)

// NDCHttpSettings represent global settings of the HTTP API, including base URL, headers, etc...
type NDCHttpSettings struct {
	Servers         []ServerConfig             `json:"servers"                   mapstructure:"servers"         yaml:"servers"`
	Headers         map[string]utils.EnvString `json:"headers,omitempty"         mapstructure:"headers"         yaml:"headers,omitempty"`
	ArgumentPresets []ArgumentPresetConfig     `json:"argumentPresets,omitempty" mapstructure:"argumentPresets" yaml:"argumentPresets,omitempty"`
	SecuritySchemes map[string]SecurityScheme  `json:"securitySchemes,omitempty" mapstructure:"securitySchemes" yaml:"securitySchemes,omitempty"`
	Security        AuthSecurities             `json:"security,omitempty"        mapstructure:"security"        yaml:"security,omitempty"`
	Version         string                     `json:"version,omitempty"         mapstructure:"version"         yaml:"version,omitempty"`
	TLS             *TLSConfig                 `json:"tls,omitempty"             mapstructure:"tls"             yaml:"tls,omitempty"`
}

// Validate if the current instance is valid
func (rs *NDCHttpSettings) Validate() error {
	for _, server := range rs.Servers {
		if err := server.Validate(); err != nil {
			return err
		}
	}

	for key, scheme := range rs.SecuritySchemes {
		if err := scheme.Validate(); err != nil {
			return fmt.Errorf("securityScheme %s: %w", key, err)
		}
	}

	for i, preset := range rs.ArgumentPresets {
		if _, _, err := preset.Validate(); err != nil {
			return fmt.Errorf("argumentPresets[%d]: %w", i, err)
		}
	}

	if rs.TLS != nil {
		if err := rs.TLS.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// ServerConfig contains server configurations.
type ServerConfig struct {
	URL             utils.EnvString            `json:"url"                       mapstructure:"url"             yaml:"url"`
	ID              string                     `json:"id,omitempty"              mapstructure:"id"              yaml:"id,omitempty"`
	ArgumentPresets []ArgumentPresetConfig     `json:"argumentPresets,omitempty" mapstructure:"argumentPresets" yaml:"argumentPresets,omitempty"`
	Headers         map[string]utils.EnvString `json:"headers,omitempty"         mapstructure:"headers"         yaml:"headers,omitempty"`
	SecuritySchemes map[string]SecurityScheme  `json:"securitySchemes,omitempty" mapstructure:"securitySchemes" yaml:"securitySchemes,omitempty"`
	Security        AuthSecurities             `json:"security,omitempty"        mapstructure:"security"        yaml:"security,omitempty"`
	TLS             *TLSConfig                 `json:"tls,omitempty"             mapstructure:"tls"             yaml:"tls,omitempty"`
}

// Validate if the current instance is valid
func (ss *ServerConfig) Validate() error {
	rawURL, err := ss.URL.Get()
	if err != nil {
		return fmt.Errorf("server url: %w", err)
	}

	if rawURL == "" {
		return errors.New("url is required for server")
	}

	_, err = parseHttpURL(rawURL)
	if err != nil {
		return fmt.Errorf("server url: %w", err)
	}

	if ss.TLS != nil {
		if err := ss.TLS.Validate(); err != nil {
			return fmt.Errorf("tls: %w", err)
		}
	}

	return nil
}

// Validate if the current instance is valid
func (ss ServerConfig) GetURL() (*url.URL, error) {
	rawURL, err := ss.URL.Get()
	if err != nil {
		return nil, err
	}
	urlValue, err := parseHttpURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("server url: %w", err)
	}

	return urlValue, nil
}

// ArgumentPresetConfig represents an argument preset configuration.
type ArgumentPresetConfig struct {
	// The JSON path of the argument field.
	Path string `json:"path" mapstructure:"path" yaml:"path"`
	// The value to be set.
	Value ArgumentPresetValue `json:"value" mapstructure:"value" yaml:"value"`
	// Target operations to be applied.
	Targets []string `json:"targets" mapstructure:"targets" yaml:"targets"`
}

// Validate checks if the configuration is valid.
func (apc ArgumentPresetConfig) Validate() (*jsonpath.Path, []regexp.Regexp, error) {
	if apc.Path == "" {
		return nil, nil, errors.New("require path in ArgumentPresetConfig")
	}

	if apc.Value.inner == nil {
		return nil, nil, errors.New("require value in ArgumentPresetConfig")
	}

	rawPath := apc.Path
	switch rawPath[0] {
	case '$':
	case '.':
		rawPath = "$" + rawPath
	default:
		rawPath = "$." + rawPath
	}

	jsonPath, err := jsonpath.Parse(rawPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse the json path: %w", err)
	}

	if len(jsonPath.Query().Segments()) == 0 {
		return nil, nil, errors.New("json path in ArgumentPresetConfig is empty")
	}

	firstSegment := jsonPath.Query().Segments()[0]
	if firstSegment.IsDescendant() {
		return nil, nil, errors.New("invalid json path. It should be selected the root field")
	}

	if selector, ok := firstSegment.Selectors()[0].(spec.Name); !ok || selector.String() == "" {
		return nil, nil, errors.New("invalid json path. The root selector must be an object name")
	}

	targets := make([]regexp.Regexp, len(apc.Targets))
	for i, target := range apc.Targets {
		rg, err := regexp.Compile(target)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to compile argument preset target expression %s: %w", target, err)
		}
		targets[i] = *rg
	}

	return jsonPath, targets, nil
}

// ArgumentPresetValue represents an argument preset value type.
type ArgumentPresetValueType string

const (
	ArgumentPresetValueTypeLiteral       ArgumentPresetValueType = "literal"
	ArgumentPresetValueTypeEnv           ArgumentPresetValueType = "env"
	ArgumentPresetValueTypeForwardHeader ArgumentPresetValueType = "forwardHeader"
)

var argumentPresetValueType_enums = []ArgumentPresetValueType{
	ArgumentPresetValueTypeLiteral,
	ArgumentPresetValueTypeEnv,
	ArgumentPresetValueTypeForwardHeader,
}

// JSONSchema is used to generate a custom jsonschema
func (j ArgumentPresetValueType) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: toAnySlice(argumentPresetValueType_enums),
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ArgumentPresetValueType) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseArgumentPresetValueType(rawResult)
	if err != nil {
		return err
	}

	*j = result

	return nil
}

// ParseArgumentPresetValueType parses ArgumentPresetValueType from string
func ParseArgumentPresetValueType(value string) (ArgumentPresetValueType, error) {
	result := ArgumentPresetValueType(value)
	if !slices.Contains(argumentPresetValueType_enums, result) {
		return result, fmt.Errorf("invalid ArgumentPresetValueType. Expected %+v, got <%s>", argumentPresetValueType_enums, value)
	}

	return result, nil
}

// ArgumentPresetValue represents an argument preset value information.
type ArgumentPresetValue struct {
	inner ArgumentPresetValueInterface
}

// Interface returns the inner interface.
func (j ArgumentPresetValue) Interface() ArgumentPresetValueInterface {
	return j.inner
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ArgumentPresetValue) UnmarshalJSON(b []byte) error {
	var rawValue map[string]any
	if err := json.Unmarshal(b, &rawValue); err != nil {
		return err
	}

	strType, err := getStringFromAnyMap(rawValue, "type")
	if err != nil {
		return fmt.Errorf("ArgumentPresetValue.type: %w", err)
	}

	valueType, err := ParseArgumentPresetValueType(strType)
	if err != nil {
		return fmt.Errorf("ArgumentPresetValue.type: %w", err)
	}

	switch valueType {
	case ArgumentPresetValueTypeLiteral:
		j.inner = &ArgumentPresetValueLiteral{
			Type:  valueType,
			Value: rawValue["value"],
		}
	case ArgumentPresetValueTypeEnv:
		name, err := getStringFromAnyMap(rawValue, "name")
		if err != nil {
			return fmt.Errorf("ArgumentPresetValue.name: %w", err)
		}

		j.inner = &ArgumentPresetValueEnv{
			Type: valueType,
			Name: name,
		}
	case ArgumentPresetValueTypeForwardHeader:
		name, err := getStringFromAnyMap(rawValue, "name")
		if err != nil {
			return fmt.Errorf("ArgumentPresetValue.name: %w", err)
		}

		j.inner = &ArgumentPresetValueForwardHeader{
			Type: valueType,
			Name: name,
		}
	}

	return nil
}

// MarshalJSON implements json.Marshaler.
func (j ArgumentPresetValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.inner)
}

// JSONSchema is used to generate a custom jsonschema
func (j ArgumentPresetValue) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			ArgumentPresetValueLiteral{}.JSONSchema(),
			ArgumentPresetValueEnv{}.JSONSchema(),
			ArgumentPresetValueForwardHeader{}.JSONSchema(),
		},
	}
}

// ArgumentPresetValueInterface abstracts an interface for ArgumentPresetValue.
type ArgumentPresetValueInterface interface {
	GetType() ArgumentPresetValueType
}

// ArgumentPresetValueLiteral represents an literal argument preset value.
type ArgumentPresetValueLiteral struct {
	Type  ArgumentPresetValueType `json:"type"  mapstructure:"type"  yaml:"type"`
	Value any                     `json:"value" mapstructure:"value" yaml:"value"`
}

// JSONSchema is used to generate a custom jsonschema
func (j ArgumentPresetValueLiteral) JSONSchema() *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()
	properties.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{ArgumentPresetValueTypeLiteral},
	})

	properties.Set("value", &jsonschema.Schema{})

	return &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
		Required:   []string{"type", "value"},
	}
}

// GetType gets the type of the current argument preset value.
func (apv ArgumentPresetValueLiteral) GetType() ArgumentPresetValueType {
	return apv.Type
}

// ArgumentPresetValueEnv represents an environment argument preset value.
type ArgumentPresetValueEnv struct {
	Type ArgumentPresetValueType `json:"type" mapstructure:"type" yaml:"type"`
	Name string                  `json:"name" mapstructure:"name" yaml:"name"`
}

// JSONSchema is used to generate a custom jsonschema
func (j ArgumentPresetValueEnv) JSONSchema() *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()
	properties.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{ArgumentPresetValueTypeEnv},
	})

	properties.Set("name", &jsonschema.Schema{
		Type: "string",
	})

	return &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
		Required:   []string{"type", "name"},
	}
}

// GetType gets the type of the current argument preset value.
func (j ArgumentPresetValueEnv) GetType() ArgumentPresetValueType {
	return j.Type
}

// ArgumentPresetValueForwardHeader represents an argument preset value config from header forwarding.
type ArgumentPresetValueForwardHeader struct {
	Type ArgumentPresetValueType `json:"type" mapstructure:"type" yaml:"type"`
	Name string                  `json:"name" mapstructure:"name" yaml:"name"`
}

// JSONSchema is used to generate a custom jsonschema
func (j ArgumentPresetValueForwardHeader) JSONSchema() *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()
	properties.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{ArgumentPresetValueTypeForwardHeader},
	})

	properties.Set("name", &jsonschema.Schema{
		Type: "string",
	})

	return &jsonschema.Schema{
		Type:       "object",
		Properties: properties,
		Required:   []string{"type", "name"},
	}
}

// GetType gets the type of the current argument preset value.
func (apv ArgumentPresetValueForwardHeader) GetType() ArgumentPresetValueType {
	return apv.Type
}

// parseHttpURL parses and validate if the URL has HTTP scheme
func parseHttpURL(input string) (*url.URL, error) {
	if !strings.HasPrefix(input, "https://") && !strings.HasPrefix(input, "http://") {
		return nil, errors.New("invalid HTTP URL " + input)
	}

	return url.Parse(input)
}

func ParseRelativeOrHttpURL(input string) (*url.URL, error) {
	if strings.HasPrefix(input, "/") {
		return &url.URL{Path: input}, nil
	}

	return parseHttpURL(input)
}

func getStringFromAnyMap(input map[string]any, key string) (string, error) {
	rawValue, ok := input[key]
	if !ok {
		return "", errors.New("field is required")
	}

	strValue, ok := rawValue.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got: %v", rawValue)
	}

	return strValue, nil
}
