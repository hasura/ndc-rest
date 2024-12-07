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
	return &ArgumentPresets{
		httpSchema: httpSchema,
		presets:    nil,
	}, nil
}

// ApplyArgumentPresents replace argument preset values into request arguments.
func (ap ArgumentPresets) Apply(operationName string, arguments map[string]any) (map[string]any, error) {
	for _, preset := range ap.presets {
		if len(preset.Targets) > 0 {
			if _, ok := preset.Targets[operationName]; !ok {
				continue
			}
		}

		var err error
		arguments, err = ap.Apply(operationName, arguments)
		if err != nil {
			return nil, fmt.Errorf("failed to apply argument preset: %w", err)
		}
	}

	return arguments, nil
}
