package utils

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	nonAlphaDigitRegex = regexp.MustCompile(`[^\w]+`)
)

// ToCamelCase convert a string to camelCase
func ToCamelCase(input string) string {
	pascalCase := ToPascalCase(input)
	if pascalCase == "" {
		return pascalCase
	}
	return strings.ToLower(pascalCase[:1]) + pascalCase[1:]
}

// StringSliceToCamelCase convert a slice of strings to camelCase
func StringSliceToCamelCase(inputs []string) string {
	return fmt.Sprintf("%s%s", ToCamelCase(inputs[0]), StringSliceToPascalCase(inputs[1:]))
}

// ToPascalCase convert a string to PascalCase
func ToPascalCase(input string) string {
	if input == "" {
		return input
	}
	input = nonAlphaDigitRegex.ReplaceAllString(input, "_")
	parts := strings.Split(input, "_")
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

// stringSliceToCase convert a slice of string with a transform function
func stringSliceToCase(inputs []string, convert func(string) string, sep string) string {
	if len(inputs) == 0 {
		return ""
	}

	var results []string
	for _, item := range inputs {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		results = append(results, convert(trimmed))
	}
	return strings.Join(results, sep)
}

// StringSliceToPascalCase convert a slice of string to PascalCase
func StringSliceToPascalCase(inputs []string) string {
	return stringSliceToCase(inputs, ToPascalCase, "")
}

// ToSnakeCase converts string to snake_case
func ToSnakeCase(input string) string {
	var sb strings.Builder
	inputLen := len(input)
	for i := range inputLen {
		char := rune(input[i])
		if char == '_' || char == '-' {
			sb.WriteRune('_')
			continue
		}
		if unicode.IsDigit(char) || unicode.IsLower(char) {
			sb.WriteRune(char)
			continue
		}

		if unicode.IsUpper(char) {
			if i == 0 {
				sb.WriteRune(unicode.ToLower(char))
				continue
			}
			prevChar := rune(input[i-1])
			if unicode.IsDigit(prevChar) || unicode.IsLower(prevChar) {
				sb.WriteRune('_')
				sb.WriteRune(unicode.ToLower(char))
				continue
			}
			if i < inputLen-1 {
				nextChar := rune(input[i+1])
				if unicode.IsUpper(prevChar) && unicode.IsLetter(nextChar) && !unicode.IsUpper(nextChar) {
					sb.WriteRune('_')
					sb.WriteRune(unicode.ToLower(char))
					continue
				}
			}

			sb.WriteRune(unicode.ToLower(char))
		}
	}
	return sb.String()
}

// StringSliceToSnakeCase convert a slice of string to snake_case
func StringSliceToSnakeCase(inputs []string) string {
	return stringSliceToCase(inputs, ToSnakeCase, "_")
}

// ToConstantCase converts string to CONSTANT_CASE
func ToConstantCase(input string) string {
	return strings.ToUpper(ToSnakeCase(input))
}

// StringSliceToConstantCase convert a slice of string to CONSTANT_CASE
func StringSliceToConstantCase(inputs []string) string {
	return strings.ToUpper(StringSliceToSnakeCase(inputs))
}

// SplitStrings wrap strings.Split with all leading and trailing white space removed
func SplitStringsAndTrimSpaces(input string, sep string) []string {
	var results []string
	items := strings.Split(input, sep)
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		results = append(results, trimmed)
	}

	return results
}
