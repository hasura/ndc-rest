package contenttype

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/hasura/ndc-sdk-go/schema"
)

// StringifySimpleScalar converts a simple scalar value to string.
func StringifySimpleScalar(val reflect.Value, kind reflect.Kind) (string, error) {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(val.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(val.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(val.Float(), 'g', -1, val.Type().Bits()), nil
	case reflect.String:
		return val.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(val.Bool()), nil
	case reflect.Interface:
		return fmt.Sprint(val.Interface()), nil
	default:
		value := val.Interface()
		if stringer, ok := value.(fmt.Stringer); ok {
			return stringer.String(), nil
		}

		j, err := json.Marshal(value)
		if err != nil {
			return "", err
		}

		return string(j), nil
	}
}

// UnwrapNullableType unwraps the underlying type of the nullable type
func UnwrapNullableType(input schema.Type) (schema.TypeEncoder, bool, error) {
	switch ty := input.Interface().(type) {
	case *schema.NullableType:
		childType, _, err := UnwrapNullableType(ty.UnderlyingType)
		if err != nil {
			return nil, false, err
		}

		return childType, true, nil
	case *schema.NamedType, *schema.ArrayType:
		return ty, false, nil
	default:
		return nil, false, fmt.Errorf("invalid type %v", input)
	}
}
