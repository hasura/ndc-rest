package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gopkg.in/yaml.v3"
)

var (
	errEmptyInput           = errors.New("empty input")
	errUnknownPatchStrategy = errors.New("unable to detect patch strategy")
)

// PatchStrategy represents the patch strategy enum
type PatchStrategy string

const (
	// PatchStrategyMerge the merge strategy enum for [RFC 7396] specification
	//
	// [RFC 7396]: https://datatracker.ietf.org/doc/html/rfc7396
	PatchStrategyMerge PatchStrategy = "merge"
	// PatchStrategyJSON6902 the patch strategy enum for [RFC 6902] specification
	//
	// [RFC 6902]: https://datatracker.ietf.org/doc/html/rfc6902
	PatchStrategyJSON6902 PatchStrategy = "json6902"
)

// PatchConfig the configuration for JSON patch
type PatchConfig struct {
	Path     string        `json:"path"     yaml:"path"`
	Strategy PatchStrategy `json:"strategy" jsonschema:"enum=merge,enum=json6902" yaml:"strategy"`
}

// ApplyPatchToHTTPSchema applies JSON patches to NDC HTTP schema and validate the output
func ApplyPatchToHTTPSchema(input *schema.NDCHttpSchema, patchFiles []PatchConfig) (*schema.NDCHttpSchema, error) {
	if len(patchFiles) == 0 {
		return input, nil
	}

	bs, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	rawResult, err := ApplyPatchFromRawJSON(bs, patchFiles)
	if err != nil {
		return nil, err
	}

	var result schema.NDCHttpSchema
	if err := json.Unmarshal(rawResult, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ApplyPatch applies patches to the raw bytes input
func ApplyPatch(input []byte, patchFiles []PatchConfig) ([]byte, error) {
	jsonInput, err := convertMaybeYAMLToJSONBytes(input)
	if err != nil {
		return nil, err
	}

	return ApplyPatchFromRawJSON(jsonInput, patchFiles)
}

// ApplyPatchFromRawJSON applies patches to the raw JSON bytes input without validation request
func ApplyPatchFromRawJSON(input []byte, patchFiles []PatchConfig) ([]byte, error) {
	for _, patchFile := range patchFiles {
		walkError := WalkFiles(patchFile.Path, func(data []byte) error {
			jsonPatch, err := convertMaybeYAMLToJSONBytes(data)
			if err != nil {
				return fmt.Errorf("%s: %w", patchFile.Path, err)
			}
			strategy := patchFile.Strategy
			if strategy == "" {
				strategy, err = guessPatchStrategy(jsonPatch)
				if err != nil {
					return fmt.Errorf("%s: %w", patchFile.Path, err)
				}
			}
			switch strategy {
			case PatchStrategyJSON6902:
				patch, err := jsonpatch.DecodePatch(jsonPatch)
				if err != nil {
					return applyPatchFromFileError(patchFile, err)
				}
				input, err = patch.Apply(input)
				if err != nil {
					return applyPatchFromFileError(patchFile, err)
				}
			case PatchStrategyMerge:
				input, err = jsonpatch.MergePatch(input, jsonPatch)
				if err != nil {
					return fmt.Errorf("failed to merge JSON patch from file %s: %w", patchFile, err)
				}
			default:
				return fmt.Errorf("invalid JSON path strategy: %s", patchFile.Strategy)
			}

			return nil
		})
		if walkError != nil {
			return nil, walkError
		}
	}

	return input, nil
}

func applyPatchFromFileError(patchCfg PatchConfig, err error) error {
	return fmt.Errorf("failed to decode patch from file %s: %w", patchCfg, err)
}

func convertMaybeYAMLToJSONBytes(input []byte) ([]byte, error) {
	runes := []byte(strings.TrimSpace(string(input)))
	if len(runes) == 0 {
		return nil, errEmptyInput
	}

	if (runes[0] == '{' && runes[len(runes)-1] == '}') || (runes[0] == '[' && runes[len(runes)-1] == ']') {
		return runes, nil
	}

	var anyOutput any
	if err := yaml.Unmarshal(input, &anyOutput); err != nil {
		return nil, fmt.Errorf("input bytes are not in either yaml or json format: %w", err)
	}

	return json.Marshal(anyOutput)
}

func guessPatchStrategy(runes []byte) (PatchStrategy, error) {
	if len(runes) == 0 {
		return "", errEmptyInput
	}

	if runes[0] == '{' && runes[len(runes)-1] == '}' {
		return PatchStrategyMerge, nil
	}
	if runes[0] == '[' && runes[len(runes)-1] == ']' {
		return PatchStrategyJSON6902, nil
	}

	return "", errUnknownPatchStrategy
}
