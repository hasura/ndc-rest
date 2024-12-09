package argument

import (
	"fmt"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
)

// ArgumentPresets manage and apply default preset values to request arguments.
type ArgumentPresets struct {
	httpSchema *rest.NDCHttpSchema
	presets    []ArgumentPreset
}

// NewArgumentPresets create a new ArgumentPresets instance.
func NewArgumentPresets(httpSchema *rest.NDCHttpSchema, presets []rest.ArgumentPresetConfig) (*ArgumentPresets, error) {
	result := &ArgumentPresets{
		httpSchema: httpSchema,
		presets:    nil,
	}
	for i, item := range presets {
		preset, err := NewArgumentPreset(httpSchema, item)
		if err != nil {
			return nil, fmt.Errorf("%d: %w", i, err)
		}

		result.presets = append(result.presets, *preset)
	}

	return result, nil
}

// ApplyArgumentPresents replace argument preset values into request arguments.
func (ap ArgumentPresets) Apply(operationName string, arguments map[string]any, headers map[string]string) (map[string]any, error) {
	for _, preset := range ap.presets {
		if len(preset.Targets) == 0 {
			continue
		}

		var err error
		arguments, err = preset.Evaluate(operationName, arguments, headers)
		if err != nil {
			return nil, fmt.Errorf("failed to apply argument preset: %w", err)
		}
	}

	return arguments, nil
}
