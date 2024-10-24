package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hasura/ndc-sdk-go/utils"
	"gopkg.in/yaml.v3"
	"gotest.tools/v3/assert"
)

func TestEnvTemplate(t *testing.T) {
	testCases := []struct {
		input       string
		expected    string
		templateStr string
		templates   []EnvTemplate
	}{
		{},
		{
			input:    "http://localhost:8080",
			expected: "http://localhost:8080",
		},
		{
			input: "{{SERVER_URL}}",
			templates: []EnvTemplate{
				NewEnvTemplate("SERVER_URL"),
			},
			templateStr: "{{SERVER_URL}}",
			expected:    "",
		},
		{
			input: "{{SERVER_URL:-http://localhost:8080}}",
			templates: []EnvTemplate{
				NewEnvTemplateWithDefault("SERVER_URL", "http://localhost:8080"),
			},
			templateStr: "{{SERVER_URL:-http://localhost:8080}}",
			expected:    "http://localhost:8080",
		},
		{
			input: "{{SERVER_URL:-}}",
			templates: []EnvTemplate{
				{
					Name:         "SERVER_URL",
					DefaultValue: utils.ToPtr(""),
				},
			},
			templateStr: "{{SERVER_URL:-}}",
			expected:    "",
		},
		{
			input: "{{SERVER_URL:-http://localhost:8080}},{{SERVER_URL:-http://localhost:8080}},{{SERVER_URL}}",
			templates: []EnvTemplate{
				{
					Name:         "SERVER_URL",
					DefaultValue: utils.ToPtr("http://localhost:8080"),
				},
				{
					Name: "SERVER_URL",
				},
			},
			templateStr: "{{SERVER_URL:-http://localhost:8080}},{{SERVER_URL}}",
			expected:    "http://localhost:8080,http://localhost:8080,",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			tmpl := FindEnvTemplate(tc.input)
			if len(tc.templates) == 0 {
				if tmpl != nil {
					t.Errorf("failed to find env template, expected nil, got %s", tmpl)
				}
			} else {
				assert.DeepEqual(t, tc.templates[0].String(), tmpl.String())

				var jTemplate EnvTemplate
				if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, tc.input)), &jTemplate); err != nil {
					t.Errorf("failed to unmarshal template from json: %s", err)
					t.FailNow()
				}
				assert.DeepEqual(t, jTemplate, *tmpl)
				bs, err := json.Marshal(jTemplate)
				if err != nil {
					t.Errorf("failed to marshal template from json: %s", err)
					t.FailNow()
				}
				assert.DeepEqual(t, tmpl.String(), strings.Trim(string(bs), `"`))

				if err := yaml.Unmarshal([]byte(fmt.Sprintf(`"%s"`, tc.input)), &jTemplate); err != nil {
					t.Errorf("failed to unmarshal template from yaml: %s", err)
					t.FailNow()
				}
				assert.DeepEqual(t, jTemplate, *tmpl)
				bs, err = yaml.Marshal(jTemplate)
				if err != nil {
					t.Errorf("failed to marshal template from yaml: %s", err)
					t.FailNow()
				}
				assert.DeepEqual(t, tmpl.String(), strings.TrimSpace(strings.ReplaceAll(string(bs), "'", "")))
			}

			templates := FindAllEnvTemplates(tc.input)
			assert.DeepEqual(t, tc.templates, templates)
			templateStrings := []string{}
			for i, item := range templates {
				assert.DeepEqual(t, tc.templates[i].String(), item.String())
				templateStrings = append(templateStrings, item.String())
			}
			assert.DeepEqual(t, tc.expected, ReplaceEnvTemplates(tc.input, templates))
			if len(templateStrings) > 0 {
				assert.DeepEqual(t, tc.templateStr, strings.Join(templateStrings, ","))
			}
		})
	}
}

func TestEnvString(t *testing.T) {
	testCases := []struct {
		input    string
		expected EnvString
	}{
		{
			input: `"{{FOO:-bar}}"`,
			expected: *NewEnvStringTemplate(EnvTemplate{
				Name:         "FOO",
				DefaultValue: utils.ToPtr("bar"),
			}),
		},
		{
			input:    `"baz"`,
			expected: *NewEnvStringValue("baz"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			var result EnvString
			if err := yaml.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected.EnvTemplate, result.EnvTemplate)
			assert.DeepEqual(t, strings.Trim(tc.input, "\""), tc.expected.String())
			bs, err := yaml.Marshal(result)
			if err != nil {
				t.Fatal(t, err)
			}
			assert.DeepEqual(t, strings.Trim(tc.input, `"`), strings.TrimSpace(strings.ReplaceAll(string(bs), "'", "")))
			result.JSONSchema()
		})
	}
}

func TestEnvInt(t *testing.T) {
	testCases := []struct {
		input    string
		expected EnvInt
	}{
		{
			input:    `400`,
			expected: EnvInt{value: utils.ToPtr[int64](400)},
		},
		{
			input:    `"400"`,
			expected: *EnvInt{}.WithValue(400),
		},
		{
			input: `"{{FOO:-401}}"`,
			expected: EnvInt{
				value: utils.ToPtr(int64(401)),
				EnvTemplate: EnvTemplate{
					Name:         "FOO",
					DefaultValue: utils.ToPtr("401"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			var result EnvInt
			if err := json.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected.EnvTemplate, result.EnvTemplate)
			assert.DeepEqual(t, tc.expected.value, result.value)

			if err := yaml.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected.EnvTemplate, result.EnvTemplate)
			assert.DeepEqual(t, tc.expected.value, result.value)
			assert.DeepEqual(t, strings.Trim(tc.input, "\""), tc.expected.String())
			bs, err := yaml.Marshal(result)
			if err != nil {
				t.Fatal(t, err)
			}
			assert.DeepEqual(t, strings.Trim(tc.input, `"`), strings.TrimSpace(strings.ReplaceAll(string(bs), "'", "")))
			result.JSONSchema()
		})
	}
}

func TestEnvInts(t *testing.T) {
	testCases := []struct {
		input        string
		expected     EnvInts
		expectedYaml string
	}{
		{
			input:    `[400, 401, 403]`,
			expected: EnvInts{value: []int64{400, 401, 403}},
			expectedYaml: `- 400
- 401
- 403`,
		},
		{
			input:    `"400, 401, 403"`,
			expected: *NewEnvIntsValue(nil).WithValue([]int64{400, 401, 403}),
			expectedYaml: `- 400
- 401
- 403`,
		},
		{
			input: `"{{FOO:-400, 401, 403}}"`,
			expected: EnvInts{
				value: []int64{400, 401, 403},
				EnvTemplate: EnvTemplate{
					Name:         "FOO",
					DefaultValue: utils.ToPtr("400, 401, 403"),
				},
			},
			expectedYaml: `{{FOO:-400, 401, 403}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			var result EnvInts
			if err := json.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected.EnvTemplate, result.EnvTemplate)
			assert.DeepEqual(t, tc.expected.value, result.value)

			if err := yaml.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected.String(), result.String())
			assert.DeepEqual(t, tc.expected.value, result.value)
			bs, err := yaml.Marshal(result)
			if err != nil {
				t.Fatal(t, err)
			}
			assert.DeepEqual(t, tc.expectedYaml, strings.TrimSpace(strings.ReplaceAll(string(bs), "'", "")))
			result.JSONSchema()
		})
	}
}

func TestEnvBoolean(t *testing.T) {
	t.Setenv("TEST_BOOL", "false")
	testCases := []struct {
		input    string
		expected EnvBoolean
	}{
		{
			input:    `false`,
			expected: *NewEnvBooleanValue(false),
		},
		{
			input:    `"true"`,
			expected: *NewEnvBooleanValue(true),
		},
		{
			input: `"{{FOO:-true}}"`,
			expected: EnvBoolean{
				value: utils.ToPtr(true),
				EnvTemplate: EnvTemplate{
					Name:         "FOO",
					DefaultValue: utils.ToPtr("true"),
				},
			},
		},
		{
			input: fmt.Sprintf(`"%s"`, NewEnvBooleanTemplate(EnvTemplate{
				Name: "TEST_BOOL",
			}).String()),
			expected: EnvBoolean{
				value: utils.ToPtr(false),
				EnvTemplate: EnvTemplate{
					Name: "TEST_BOOL",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			var result EnvBoolean
			if err := json.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected.EnvTemplate, result.EnvTemplate)
			assert.DeepEqual(t, tc.expected.value, result.value)

			if err := yaml.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected.EnvTemplate, result.EnvTemplate)
			assert.DeepEqual(t, tc.expected.value, result.value)
			assert.DeepEqual(t, strings.Trim(tc.input, "\""), tc.expected.String())
			bs, err := yaml.Marshal(result)
			if err != nil {
				t.Fatal(t, err)
			}
			assert.DeepEqual(t, strings.Trim(tc.input, `"`), strings.TrimSpace(strings.ReplaceAll(string(bs), "'", "")))
			result.JSONSchema()
			if err = (&EnvBoolean{}).UnmarshalText([]byte(strings.Trim(tc.input, `"`))); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
		})
	}
}

func TestEnvStrings(t *testing.T) {
	t.Setenv("TEST_STRINGS", "a,b,c")
	testCases := []struct {
		input        string
		expected     EnvStrings
		expectedYaml string
	}{
		{
			input:    `["foo", "bar"]`,
			expected: *NewEnvStringsValue([]string{"foo", "bar"}),
			expectedYaml: `- foo
- bar`,
		},
		{
			input:    `"foo, baz"`,
			expected: *NewEnvStringsValue(nil).WithValue([]string{"foo", "baz"}),
			expectedYaml: `- foo
- baz`,
		},
		{
			input: fmt.Sprintf(`"%s"`, NewEnvStringsTemplate(NewEnvTemplate("TEST_STRINGS")).String()),
			expected: EnvStrings{
				value: []string{"a", "b", "c"},
				EnvTemplate: EnvTemplate{
					Name: "TEST_STRINGS",
				},
			},
			expectedYaml: `{{TEST_STRINGS}}`,
		},
		{
			input: `"{{FOO:-foo, bar}}"`,
			expected: EnvStrings{
				value: []string{"foo", "bar"},
				EnvTemplate: EnvTemplate{
					Name:         "FOO",
					DefaultValue: utils.ToPtr("foo, bar"),
				},
			},
			expectedYaml: `{{FOO:-foo, bar}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			var result EnvStrings
			if err := json.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected.EnvTemplate, result.EnvTemplate)
			assert.DeepEqual(t, tc.expected.value, result.value)

			if err := yaml.Unmarshal([]byte(tc.input), &result); err != nil {
				t.Error(t, err)
				t.FailNow()
			}
			assert.DeepEqual(t, tc.expected.String(), result.String())
			assert.DeepEqual(t, tc.expected.value, result.value)
			bs, err := yaml.Marshal(result)
			if err != nil {
				t.Fatal(t, err)
			}
			assert.DeepEqual(t, tc.expectedYaml, strings.TrimSpace(strings.ReplaceAll(string(bs), "'", "")))
			result.JSONSchema()
		})
	}
}
