package internal

import (
	"fmt"

	"github.com/hasura/ndc-sdk-go/schema"
)

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
