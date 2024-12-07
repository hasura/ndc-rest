package argument

import (
	"errors"

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
	jsonPath, targetExpressions, err := preset.Validate()
	if err != nil {
		return nil, err
	}

	targets := make(map[string]schema.TypeRepresentation)
	for _, expr := range targetExpressions {
		for key := range httpSchema.Functions {
			if expr.MatchString(key) {
				targets[key] = schema.NewTypeRepresentationString().Encode()
			}
		}

		for key := range httpSchema.Procedures {
			if expr.MatchString(key) {
				targets[key] = schema.NewTypeRepresentationString().Encode()
			}
		}
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
	firstSegment := ap.Path.Query().Segments()[0]
	selectors := firstSegment.Selectors()

	selector, ok := selectors[0].(*spec.Name)
	if !ok || selector.String() == "" {
		return nil, errors.New("invalid json path. The root selector must be an object name")
	}

	if len(selectors) == 1 {
		value, err := ap.Value.GetValue(headers, ap.getTypeRepresentation(operationName))
		if err != nil {
			return nil, err
		}

		arguments[selector.String()] = value
	}

	// switch t := segments[0].Selectors().(type) {
	// case *spec.Name:
	// }

	return arguments, nil
}

func (ap ArgumentPreset) getTypeRepresentation(operationName string) schema.TypeRepresentation {
	if rep, ok := ap.Targets[operationName]; ok {
		return rep
	}

	return nil
}
