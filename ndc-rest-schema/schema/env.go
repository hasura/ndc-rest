package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/invopop/jsonschema"
	"gopkg.in/yaml.v3"
)

var envVariableRegex = regexp.MustCompile(`{{([A-Z0-9_]+)(:-([^}]*))?}}`)

// EnvTemplate represents an environment variable template
type EnvTemplate struct {
	Name         string
	DefaultValue *string
}

// NewEnvTemplate creates an EnvTemplate without default value
func NewEnvTemplate(name string) EnvTemplate {
	return EnvTemplate{
		Name: name,
	}
}

// NewEnvTemplateWithDefault creates an EnvTemplate with a default value
func NewEnvTemplateWithDefault(name string, defaultValue string) EnvTemplate {
	return EnvTemplate{
		Name:         name,
		DefaultValue: &defaultValue,
	}
}

// IsEmpty checks if env template is empty
func (et EnvTemplate) IsEmpty() bool {
	return et.Name == ""
}

// Value returns the value which is retrieved from system or the default value if exist
func (et EnvTemplate) Value() (string, bool) {
	value, ok := os.LookupEnv(et.Name)
	if !ok && et.DefaultValue != nil {
		return *et.DefaultValue, true
	}
	return value, ok
}

// String implements the Stringer interface
func (et EnvTemplate) String() string {
	if et.IsEmpty() {
		return ""
	}
	if et.DefaultValue == nil {
		return fmt.Sprintf("{{%s}}", et.Name)
	}
	return fmt.Sprintf("{{%s:-%s}}", et.Name, *et.DefaultValue)
}

// MarshalJSON implements json.Marshaler.
func (j EnvTemplate) MarshalJSON() ([]byte, error) {
	if j.IsEmpty() {
		return json.Marshal(nil)
	}
	return json.Marshal(j.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *EnvTemplate) UnmarshalJSON(b []byte) error {
	var rawValue string
	if err := json.Unmarshal(b, &rawValue); err != nil {
		return err
	}

	value := FindEnvTemplate(rawValue)
	if value != nil {
		*j = *value
	}
	return nil
}

// MarshalYAML implements yaml.Marshaler interface
func (j EnvTemplate) MarshalYAML() (any, error) {
	if j.IsEmpty() {
		return yaml.Marshal(nil)
	}
	return j.String(), nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface
func (j *EnvTemplate) UnmarshalYAML(node *yaml.Node) error {
	if node.Value == "" {
		return nil
	}
	value := FindEnvTemplate(node.Value)
	if value != nil {
		*j = *value
	}
	return nil
}

// FindEnvTemplate finds one environment template from string
func FindEnvTemplate(input string) *EnvTemplate {
	matches := envVariableRegex.FindStringSubmatch(input)
	return parseEnvTemplateFromMatches(matches)
}

// FindAllEnvTemplates finds all unique environment templates from string
func FindAllEnvTemplates(input string) []EnvTemplate {
	matches := envVariableRegex.FindAllStringSubmatch(input, -1)
	var results []EnvTemplate
	for _, item := range matches {
		env := parseEnvTemplateFromMatches(item)
		if env == nil {
			continue
		}
		doesExist := false
		for _, result := range results {
			if env.String() == result.String() {
				doesExist = true
				break
			}
		}
		if !doesExist {
			results = append(results, *env)
		}
	}
	return results
}

func parseEnvTemplateFromMatches(matches []string) *EnvTemplate {
	if len(matches) != 4 {
		return nil
	}
	result := &EnvTemplate{
		Name: matches[1],
	}

	if matches[2] != "" {
		result.DefaultValue = &matches[3]
	}
	return result
}

// ReplaceEnvTemplates replaces env templates in the input string with values
func ReplaceEnvTemplates(input string, envTemplates []EnvTemplate) string {
	for _, env := range envTemplates {
		value, _ := env.Value()
		input = strings.ReplaceAll(input, env.String(), value)
	}
	return input
}

// EnvString implements the environment encoding and decoding value
type EnvString struct {
	value *string
	EnvTemplate
}

// JSONSchema is used to generate a custom jsonschema
func (j EnvString) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
	}
}

// WithValue returns a new EnvString instance with new value
func (j EnvString) WithValue(value string) *EnvString {
	j.value = &value
	return &j
}

// Value returns the value which is retrieved from system or the default value if exist
func (et *EnvString) Value() *string {
	if et.value != nil {
		v := *et.value
		return &v
	}

	strValue, ok := et.EnvTemplate.Value()
	if !ok && strValue == "" {
		return nil
	}

	if ok {
		et.value = &strValue
	}
	copyVal := strValue
	return &copyVal
}

// Equal checks if the current value equals the target
func (et EnvString) Equal(target EnvString) bool {
	srcValue := et.Value()
	targetValue := target.Value()

	return (srcValue == nil && targetValue == nil) ||
		(srcValue != nil && targetValue != nil && *srcValue == *targetValue)
}

// String implements the Stringer interface
func (et EnvString) String() string {
	if et.IsEmpty() {
		if et.value == nil {
			return ""
		}
		return *et.value
	}
	return et.EnvTemplate.String()
}

// MarshalJSON implements json.Marshaler.
func (j EnvString) MarshalJSON() ([]byte, error) {
	if j.EnvTemplate.IsEmpty() {
		return json.Marshal(j.value)
	}
	return j.EnvTemplate.MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *EnvString) UnmarshalJSON(b []byte) error {
	var rawValue string
	if err := json.Unmarshal(b, &rawValue); err != nil {
		return err
	}

	return j.unmarshalText(rawValue)
}

// MarshalYAML implements yaml.Marshaler interface
func (j EnvString) MarshalYAML() (any, error) {
	if j.EnvTemplate.IsEmpty() {
		return j.value, nil
	}
	return j.EnvTemplate.MarshalYAML()
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (j *EnvString) UnmarshalYAML(node *yaml.Node) error {
	if node.Value == "" {
		return nil
	}
	return j.unmarshalText(node.Value)
}

// UnmarshalText decodes the integer slice from string
func (j *EnvString) UnmarshalText(text []byte) error {
	return j.unmarshalText(string(text))
}

func (j *EnvString) unmarshalText(rawValue string) error {
	value := FindEnvTemplate(rawValue)
	if value != nil {
		j.EnvTemplate = *value
		j.Value()
	} else {
		j.value = &rawValue
	}
	return nil
}

// NewEnvStringValue creates an EnvString from value
func NewEnvStringValue(value string) *EnvString {
	return &EnvString{
		value: &value,
	}
}

// NewEnvStringTemplate creates an EnvString from template
func NewEnvStringTemplate(template EnvTemplate) *EnvString {
	return &EnvString{
		EnvTemplate: template,
	}
}

// EnvInt implements the integer environment encoder and decoder
type EnvInt struct {
	value *int64
	EnvTemplate
}

// NewEnvIntValue creates an EnvInt from value
func NewEnvIntValue(value int64) *EnvInt {
	return &EnvInt{
		value: &value,
	}
}

// NewEnvIntTemplate creates an EnvInt from template
func NewEnvIntTemplate(template EnvTemplate) *EnvInt {
	return &EnvInt{
		EnvTemplate: template,
	}
}

// JSONSchema is used to generate a custom jsonschema
func (j EnvInt) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "integer"},
			{Type: "string"},
		},
	}
}

// WithValue returns a new EnvInt instance with new value
func (j EnvInt) WithValue(value int64) *EnvInt {
	j.value = &value
	return &j
}

// Equal checks if the current value equals the target
func (et EnvInt) Equal(target EnvInt) bool {
	srcValue, err := et.Value()
	if err != nil {
		return false
	}
	targetValue, err := target.Value()
	if err != nil {
		return false
	}

	return (srcValue == nil && targetValue == nil) ||
		(srcValue != nil && targetValue != nil && *srcValue == *targetValue)
}

// String implements the Stringer interface
func (et EnvInt) String() string {
	if et.IsEmpty() {
		if et.value == nil {
			return ""
		}
		return strconv.FormatInt(*et.value, 10)
	}
	return et.EnvTemplate.String()
}

// MarshalJSON implements json.Marshaler.
func (j EnvInt) MarshalJSON() ([]byte, error) {
	if j.EnvTemplate.IsEmpty() {
		return json.Marshal(j.value)
	}
	return j.EnvTemplate.MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *EnvInt) UnmarshalJSON(b []byte) error {
	var v int64
	if err := json.Unmarshal(b, &v); err == nil {
		j.value = &v
		return nil
	}

	var rawValue string
	if err := json.Unmarshal(b, &rawValue); err != nil {
		return err
	}

	return j.unmarshalText(rawValue)
}

// MarshalYAML implements yaml.Marshaler interface
func (j EnvInt) MarshalYAML() (any, error) {
	if j.EnvTemplate.IsEmpty() {
		return j.value, nil
	}
	return j.EnvTemplate.MarshalYAML()
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (j *EnvInt) UnmarshalYAML(node *yaml.Node) error {
	if node.Value == "" {
		return nil
	}
	return j.unmarshalText(node.Value)
}

// UnmarshalText decodes the integer slice from string
func (j *EnvInt) UnmarshalText(text []byte) error {
	return j.unmarshalText(string(text))
}

func (j *EnvInt) unmarshalText(rawValue string) error {
	value := FindEnvTemplate(rawValue)
	if value != nil {
		j.EnvTemplate = *value
		_, err := j.Value()
		return err
	}
	if rawValue != "" {
		intValue, err := strconv.ParseInt(rawValue, 10, 64)
		if err != nil {
			return err
		}

		j.value = &intValue
	}

	return nil
}

// Value returns the value which is retrieved from system or the default value if exist
func (et *EnvInt) Value() (*int64, error) {
	if et.value != nil {
		v := *et.value
		return &v, nil
	}

	strValue, ok := et.EnvTemplate.Value()
	if !ok && strValue == "" {
		return nil, nil
	}

	intValue, err := strconv.ParseInt(strValue, 10, 64)
	if err != nil {
		return nil, err
	}

	if ok {
		et.value = &intValue
	}

	copyVal := intValue
	return &copyVal, nil
}

// EnvInts implements the integer environment encoder and decoder
type EnvInts struct {
	value []int64
	EnvTemplate
}

// NewEnvIntsValue creates EnvInts from value
func NewEnvIntsValue(value []int64) *EnvInts {
	return &EnvInts{
		value: value,
	}
}

// NewEnvIntsTemplate creates EnvInts from template
func NewEnvIntsTemplate(template EnvTemplate) *EnvInts {
	return &EnvInts{
		EnvTemplate: template,
	}
}

// JSONSchema is used to generate a custom jsonschema
func (j EnvInts) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "string"},
			{Type: "array", Items: &jsonschema.Schema{Type: "integer"}},
		},
	}
}

// WithValue returns a new EnvInts instance with new value
func (j EnvInts) WithValue(value []int64) *EnvInts {
	j.value = value
	return &j
}

// Equal checks if the current value equals the target
func (et EnvInts) Equal(target EnvInts) bool {
	srcValue, err := et.Value()
	if err != nil {
		return false
	}
	targetValue, err := target.Value()
	if err != nil {
		return false
	}

	return slices.Equal(srcValue, targetValue)
}

// String implements the Stringer interface
func (et EnvInts) String() string {
	if et.IsEmpty() {
		return fmt.Sprintf("%v", et.value)
	}
	return et.EnvTemplate.String()
}

// MarshalJSON implements json.Marshaler.
func (j EnvInts) MarshalJSON() ([]byte, error) {
	if j.EnvTemplate.IsEmpty() {
		return json.Marshal(j.value)
	}
	return j.EnvTemplate.MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *EnvInts) UnmarshalJSON(b []byte) error {
	var v []int64
	if err := json.Unmarshal(b, &v); err == nil {
		j.value = v
		return nil
	}

	var rawValue string
	if err := json.Unmarshal(b, &rawValue); err != nil {
		return err
	}

	return j.unmarshalText(rawValue)
}

// MarshalYAML implements yaml.Marshaler.
func (j EnvInts) MarshalYAML() (any, error) {
	if j.EnvTemplate.IsEmpty() {
		return j.value, nil
	}
	return j.EnvTemplate.MarshalYAML()
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (j *EnvInts) UnmarshalYAML(node *yaml.Node) error {
	if node.Value == "" {
		return nil
	}

	return j.unmarshalText(node.Value)
}

// UnmarshalText decodes the integer slice from string
func (j *EnvInts) UnmarshalText(text []byte) error {
	return j.unmarshalText(string(text))
}

func (j *EnvInts) unmarshalText(rawValue string) error {
	value := FindEnvTemplate(rawValue)
	if value != nil {
		j.EnvTemplate = *value
		_, err := j.Value()
		return err
	}

	if rawValue != "" {
		intValues, err := parseIntsFromString(rawValue)
		if err != nil {
			return err
		}

		j.value = intValues
	}

	return nil
}

// Value returns the value which is retrieved from system or the default value if exist
func (et *EnvInts) Value() ([]int64, error) {
	if et.value != nil {
		return et.value, nil
	}

	strValue, ok := et.EnvTemplate.Value()
	if !ok && strValue == "" {
		return nil, nil
	}

	intValues, err := parseIntsFromString(strValue)
	if err != nil {
		return nil, err
	}
	if ok {
		et.value = intValues
	}

	return intValues, nil
}

func parseIntsFromString(input string) ([]int64, error) {
	var intValues []int64
	for _, str := range strings.Split(input, ",") {
		intValue, err := strconv.ParseInt(strings.TrimSpace(str), 10, 64)
		if err != nil {
			return nil, err
		}
		intValues = append(intValues, intValue)
	}

	return intValues, nil
}

// EnvBoolean implements the boolean environment encoder and decoder
type EnvBoolean struct {
	value *bool
	EnvTemplate
}

// NewEnvBooleanValue creates an EnvBoolean from value
func NewEnvBooleanValue(value bool) *EnvBoolean {
	return &EnvBoolean{
		value: &value,
	}
}

// NewEnvBooleanTemplate creates an EnvBoolean from template
func NewEnvBooleanTemplate(template EnvTemplate) *EnvBoolean {
	return &EnvBoolean{
		EnvTemplate: template,
	}
}

// JSONSchema is used to generate a custom jsonschema
func (j EnvBoolean) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "boolean"},
			{Type: "string"},
		},
	}
}

// WithValue returns a new EnvBoolean instance with new value
func (j EnvBoolean) WithValue(value bool) *EnvBoolean {
	j.value = &value
	return &j
}

// Equal checks if the current value equals the target
func (et EnvBoolean) Equal(target EnvBoolean) bool {
	srcValue, err := et.Value()
	if err != nil {
		return false
	}
	targetValue, err := target.Value()
	if err != nil {
		return false
	}

	return (srcValue == nil && targetValue == nil) ||
		(srcValue != nil && targetValue != nil && *srcValue == *targetValue)
}

// String implements the Stringer interface
func (et EnvBoolean) String() string {
	if et.IsEmpty() {
		if et.value == nil {
			return ""
		}
		return strconv.FormatBool(*et.value)
	}
	return et.EnvTemplate.String()
}

// MarshalJSON implements json.Marshaler.
func (j EnvBoolean) MarshalJSON() ([]byte, error) {
	if j.EnvTemplate.IsEmpty() {
		return json.Marshal(j.value)
	}
	return j.EnvTemplate.MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *EnvBoolean) UnmarshalJSON(b []byte) error {
	var v bool
	if err := json.Unmarshal(b, &v); err == nil {
		j.value = &v
		return nil
	}

	var rawValue string
	if err := json.Unmarshal(b, &rawValue); err != nil {
		return err
	}

	return j.unmarshalText(rawValue)
}

// UnmarshalText decodes boolean from string
func (j *EnvBoolean) UnmarshalText(text []byte) error {
	return j.unmarshalText(string(text))
}

func (j *EnvBoolean) unmarshalText(rawValue string) error {
	value := FindEnvTemplate(rawValue)
	if value != nil {
		j.EnvTemplate = *value
		_, err := j.Value()
		return err
	}
	if rawValue != "" {
		boolValue, err := strconv.ParseBool(rawValue)
		if err != nil {
			return err
		}

		j.value = &boolValue
	}

	return nil
}

// MarshalYAML implements yaml.Marshaler interface
func (j EnvBoolean) MarshalYAML() (any, error) {
	if j.EnvTemplate.IsEmpty() {
		return j.value, nil
	}
	return j.EnvTemplate.MarshalYAML()
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (j *EnvBoolean) UnmarshalYAML(node *yaml.Node) error {
	if node.Value == "" {
		return nil
	}
	return j.unmarshalText(node.Value)
}

// Value returns the value which is retrieved from system or the default value if exist
func (et *EnvBoolean) Value() (*bool, error) {
	if et.value != nil {
		v := *et.value
		return &v, nil
	}

	strValue, ok := et.EnvTemplate.Value()
	if !ok && strValue == "" {
		return nil, nil
	}

	boolValue, err := strconv.ParseBool(strValue)
	if err != nil {
		return nil, err
	}

	if ok {
		et.value = &boolValue
	}

	copyVal := boolValue
	return &copyVal, nil
}

// EnvStrings implements the string slice environment encoder and decoder
type EnvStrings struct {
	value []string
	EnvTemplate
}

// NewEnvStringsValue creates EnvStrings from value
func NewEnvStringsValue(value []string) *EnvStrings {
	return &EnvStrings{
		value: value,
	}
}

// NewEnvStringsTemplate creates EnvStrings from template
func NewEnvStringsTemplate(template EnvTemplate) *EnvStrings {
	return &EnvStrings{
		EnvTemplate: template,
	}
}

// JSONSchema is used to generate a custom jsonschema
func (j EnvStrings) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "string"},
			{Type: "array", Items: &jsonschema.Schema{Type: "string"}},
		},
	}
}

// WithValue returns a new EnvStrings instance with new value
func (j EnvStrings) WithValue(value []string) *EnvStrings {
	j.value = value
	return &j
}

// Equal checks if the current value equals the target
func (et EnvStrings) Equal(target EnvStrings) bool {
	srcValue, err := et.Value()
	if err != nil {
		return false
	}
	targetValue, err := target.Value()
	if err != nil {
		return false
	}

	return slices.Equal(srcValue, targetValue)
}

// String implements the Stringer interface
func (et EnvStrings) String() string {
	if et.IsEmpty() {
		return fmt.Sprintf("%v", et.value)
	}
	return et.EnvTemplate.String()
}

// MarshalJSON implements json.Marshaler.
func (j EnvStrings) MarshalJSON() ([]byte, error) {
	if j.EnvTemplate.IsEmpty() {
		return json.Marshal(j.value)
	}
	return j.EnvTemplate.MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *EnvStrings) UnmarshalJSON(b []byte) error {
	var v []string
	if err := json.Unmarshal(b, &v); err == nil {
		j.value = v
		return nil
	}

	var rawValue string
	if err := json.Unmarshal(b, &rawValue); err != nil {
		return err
	}

	return j.unmarshalText(rawValue)
}

// MarshalYAML implements yaml.Marshaler.
func (j EnvStrings) MarshalYAML() (any, error) {
	if j.EnvTemplate.IsEmpty() {
		return j.value, nil
	}
	return j.EnvTemplate.MarshalYAML()
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (j *EnvStrings) UnmarshalYAML(node *yaml.Node) error {
	if node.Value == "" {
		return nil
	}

	return j.unmarshalText(node.Value)
}

// UnmarshalText decodes the integer slice from string
func (j *EnvStrings) UnmarshalText(text []byte) error {
	return j.unmarshalText(string(text))
}

func (j *EnvStrings) unmarshalText(rawValue string) error {
	value := FindEnvTemplate(rawValue)
	if value != nil {
		j.EnvTemplate = *value
		_, err := j.Value()
		return err
	}

	if rawValue != "" {
		values := strings.Split(rawValue, ",")
		j.value = make([]string, len(values))
		for i, v := range values {
			j.value[i] = strings.TrimSpace(v)
		}
	} else {
		j.value = []string{}
	}

	return nil
}

// Value returns the value which is retrieved from system or the default value if exist
func (et *EnvStrings) Value() ([]string, error) {
	if et.value != nil {
		return et.value, nil
	}

	strValue, ok := et.EnvTemplate.Value()
	if !ok && strValue == "" {
		return nil, nil
	}

	err := et.unmarshalText(strValue)
	if err != nil {
		return nil, err
	}
	return et.value, nil
}
