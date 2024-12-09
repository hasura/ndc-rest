package argument

import (
	"errors"

	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/theory/jsonpath"
	"github.com/theory/jsonpath/spec"
)

// ArgumentPreset represents an argument preset.
type ArgumentPreset struct {
	Path    *jsonpath.Path
	Value   ArgumentPresetValueGetter
	Targets map[string]schema.TypeRepresentation
}

// NewArgumentPreset create a new ArgumentPreset instance.
func NewArgumentPreset(httpSchema *rest.NDCHttpSchema, preset rest.ArgumentPresetConfig) (*ArgumentPreset, error) {
	jsonPath, targets, err := configuration.ValidateArgumentPreset(httpSchema, preset)
	if err != nil {
		return nil, err
	}

	getter, err := NewArgumentPresetValueGetter(preset.Value)
	if err != nil {
		return nil, err
	}

	return &ArgumentPreset{
		Path:    jsonPath,
		Targets: targets,
		Value:   getter,
	}, nil
}

// Evaluate iterates and inject values into request arguments recursively.
func (ap ArgumentPreset) Evaluate(operationName string, arguments map[string]any, headers map[string]string) (map[string]any, error) {
	key := configuration.BuildArgumentPresetJSONPathKey(operationName, ap.Path)
	if _, ok := ap.Targets[key]; !ok {
		return arguments, nil
	}

	segments := ap.Path.Query().Segments()
	rootSelector, ok := segments[0].Selectors()[0].(spec.Name)
	if !ok || rootSelector == "" {
		return nil, errors.New("invalid json path. The root selector must be an object name")
	}

	value, err := ap.Value.GetValue(headers, ap.getTypeRepresentation(key))
	if err != nil {
		return nil, err
	}

	if len(segments) == 1 {
		arguments[string(rootSelector)] = value

		return arguments, nil
	}

	nestedValue, err := ap.evalNestedField(segments[1:], arguments[string(rootSelector)], value)
	if err != nil {
		return nil, err
	}

	arguments[string(rootSelector)] = nestedValue

	return arguments, nil
}

func (ap ArgumentPreset) evalNestedField(segments []*spec.Segment, argument any, value any) (any, error) {
	segmentsLen := len(segments)
	if segmentsLen == 0 || len(segments[0].Selectors()) == 0 {
		return value, nil
	}

	switch selector := segments[0].Selectors()[0].(type) {
	case spec.Name:
		argumentMap, mok := argument.(map[string]any)
		if !mok {
			argumentMap = make(map[string]any)
		}

		if segmentsLen == 1 {
			argumentMap[string(selector)] = value

			return argumentMap, nil
		}

		nestedValue, err := ap.evalNestedField(segments[1:], argumentMap[string(selector)], value)
		if err != nil {
			return nil, err
		}

		argumentMap[string(selector)] = nestedValue

		return argumentMap, nil
	default:
		return nil, errors.New("unsupported jsonpath spec: " + selector.String())
	}
}

func (ap ArgumentPreset) getTypeRepresentation(key string) schema.TypeRepresentation {
	if rep, ok := ap.Targets[key]; ok {
		return rep
	}

	return nil
}
