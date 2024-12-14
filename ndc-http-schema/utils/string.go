package utils

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	nonAlphaDigitRegex = regexp.MustCompile(`[^\w]+`)
)

const (
	htmlTagStart = 60 // Unicode `<`
	htmlTagEnd   = 62 // Unicode `>`
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
	return ToCamelCase(inputs[0]) + StringSliceToPascalCase(inputs[1:])
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

// StripHTMLTags aggressively strips HTML tags from a string. It will only keep anything between `>` and `<`.
func StripHTMLTags(str string) string {
	if str == "" {
		return ""
	}

	// Setup a string builder and allocate enough memory for the new string.
	var builder strings.Builder
	builder.Grow(len(str) + utf8.UTFMax)

	in := false // True if we are inside an HTML tag.
	start := 0  // The index of the previous start tag character `<`
	end := 0    // The index of the previous end tag character `>`

	for i, c := range str {
		// If this is the last character and we are not in an HTML tag, save it.
		if (i+1) == len(str) && end >= start {
			builder.WriteString(str[end:])
		}

		// Keep going if the character is not `<` or `>`
		if c != htmlTagStart && c != htmlTagEnd {
			continue
		}

		if c == htmlTagStart {
			// Only update the start if we are not in a tag.
			// This make sure we strip out `<<br>` not just `<br>`
			if !in {
				start = i

				// Write the valid string between the close and start of the two tags.
				builder.WriteString(str[end:start])
			}
			in = true

			continue
		}
		// else c == htmlTagEnd
		in = false
		end = i + 1
	}

	return strings.TrimSpace(builder.String())
}

// RemoveYAMLSpecialCharacters remote special characters to avoid YAML unmarshaling error
func RemoveYAMLSpecialCharacters(input []byte) string {
	if len(input) == 0 {
		return ""
	}

	var sb strings.Builder
	inputLength := len(input)
	for i := 0; i < inputLength; i++ {
		c := input[i]
		r := rune(c)
		switch {
		case unicode.IsSpace(r):
			sb.WriteRune(' ')
		case c == '\\' && i < inputLength-1:
			switch input[i+1] {
			case 'b', 'n', 'r', 't', 'f':
				sb.WriteRune(' ')
				i++
			case 'u':
				u := getu4(input[i:])
				if u > -1 {
					i += 5
					if slices.Contains([]rune{'<', '>', '&'}, u) {
						sb.WriteRune(u)
					}
				} else {
					sb.WriteRune(r)
					i++
				}
			case '\\':
				sb.WriteRune(r)
				sb.WriteRune(r)
				i++
			default:
				sb.WriteByte(c)
			}
		case !unicode.IsControl(r) && unicode.IsPrint(r) && utf8.ValidRune(r) && r != utf8.RuneError:
			sb.WriteByte(c)
		}
	}

	return strings.ToValidUTF8(sb.String(), "")
}

// getu4 decodes \uXXXX from the beginning of s, returning the hex value,
// or it returns -1. Borrow from the standard [JSON decoder] library
//
// [JSON decoder](https://github.com/golang/go/blob/0a6f05e30f58023bf45f747a79c20751db2bcfe7/src/encoding/json/decode.go#L1151)
func getu4(s []byte) rune {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return -1
	}
	var r rune
	for _, c := range s[2:6] {
		switch {
		case '0' <= c && c <= '9':
			c -= '0'
		case 'a' <= c && c <= 'f':
			c = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			c = c - 'A' + 10
		default:
			return -1
		}
		r = r*16 + rune(c)
	}

	return r
}

// MaskString masks the string value for security
func MaskString(input string) string {
	inputLength := len(input)
	switch {
	case inputLength < 6:
		return strings.Repeat("*", inputLength)
	case inputLength < 12:
		return input[0:1] + strings.Repeat("*", inputLength-1)
	default:
		return input[0:3] + strings.Repeat("*", 7) + fmt.Sprintf("(%d)", inputLength)
	}
}
