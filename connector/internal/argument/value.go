package argument

import (
	"fmt"
	"os"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

// NewArgumentPresetValueGetter creates an ArgumentPresetValueGetter from config.
func NewArgumentPresetValueGetter(presetValue rest.ArgumentPresetValue) (ArgumentPresetValueGetter, error) {
	switch t := presetValue.Interface().(type) {
	case *rest.ArgumentPresetValueLiteral:
		return NewArgumentPresetValueLiteral(t.Value), nil
	case *rest.ArgumentPresetValueEnv:
		return NewArgumentPresetValueEnv(t.Name), nil
	case *rest.ArgumentPresetValueForwardHeader:
		return NewArgumentPresetValueForwardHeader(t.Name), nil
	default:
		return nil, fmt.Errorf("unsupported argument preset value: %v", presetValue)
	}
}

// ArgumentPresetValueGetter abstracts the value getter of a argument preset.
type ArgumentPresetValueGetter interface {
	GetValue(headers map[string]string, typeRep schema.TypeRepresentation) (any, error)
}

// ArgumentPresetValueLiteral represents an argument preset getter from a literal value.
type ArgumentPresetValueLiteral struct {
	value any
}

// NewArgumentPresetValueLiteral creates a new ArgumentPresetValueLiteral instance.
func NewArgumentPresetValueLiteral(value any) *ArgumentPresetValueLiteral {
	return &ArgumentPresetValueLiteral{value: value}
}

// GetValue gets and parses the argument preset value.
func (apv ArgumentPresetValueLiteral) GetValue(_ map[string]string, typeRep schema.TypeRepresentation) (any, error) {
	return apv.value, nil
}

// ArgumentPresetValueEnv represents the argument preset getter from an environment variable.
type ArgumentPresetValueEnv struct {
	rawValue *string
}

// NewArgumentPresetValueEnv creates a new ArgumentPresetValueEnv instance.
func NewArgumentPresetValueEnv(name string) *ArgumentPresetValueEnv {
	var value *string
	rawValue, ok := os.LookupEnv(name)
	if ok {
		value = &rawValue
	}

	return &ArgumentPresetValueEnv{
		rawValue: value,
	}
}

// GetValue gets and parses the argument preset value.
func (apv ArgumentPresetValueEnv) GetValue(_ map[string]string, typeRep schema.TypeRepresentation) (any, error) {
	if apv.rawValue == nil || typeRep == nil {
		return apv.rawValue, nil
	}

	return convertTypePresentationFromString(*apv.rawValue, typeRep)
}

// ArgumentPresetValueForwardHeader represents the argument preset getter from request headers.
type ArgumentPresetValueForwardHeader struct {
	name string
}

// NewArgumentPresetValueForwardHeader creates a new ArgumentPresetValueForwardHeader instance.
func NewArgumentPresetValueForwardHeader(name string) *ArgumentPresetValueForwardHeader {
	return &ArgumentPresetValueForwardHeader{
		name: name,
	}
}

// GetValue gets and parses the argument preset value.
func (apv ArgumentPresetValueForwardHeader) GetValue(headers map[string]string, typeRep schema.TypeRepresentation) (any, error) {
	if len(headers) == 0 {
		return nil, nil
	}

	rawValue, ok := headers[apv.name]
	if !ok {
		return nil, nil
	}

	return convertTypePresentationFromString(rawValue, typeRep)
}
