package argument

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

func convertTypePresentationFromString(input string, typeRep schema.TypeRepresentation) (any, error) {
	switch t := typeRep.Interface().(type) {
	case *schema.TypeRepresentationBoolean:
		return strconv.ParseBool(input)
	case *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64:
		return strconv.ParseInt(input, 10, 64)
	case *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64:
		return strconv.ParseFloat(input, 64)
	case *schema.TypeRepresentationBigInteger:
		_, err := strconv.ParseInt(input, 10, 64)

		return input, err
	case *schema.TypeRepresentationBigDecimal:
		_, err := strconv.ParseFloat(input, 64)

		return input, err
	case *schema.TypeRepresentationString, *schema.TypeRepresentationBytes:
		return input, nil
	case *schema.TypeRepresentationEnum:
		if !slices.Contains(t.OneOf, input) {
			return nil, fmt.Errorf("invalid enum value, expected one of %v, got: %s", t.OneOf, input)
		}

		return input, nil
	case *schema.TypeRepresentationUUID:
		_, err := uuid.Parse(input)

		return input, err
	case *schema.TypeRepresentationDate:
		_, err := utils.DecodeDate(input)

		return input, err
	case *schema.TypeRepresentationTimestamp, *schema.TypeRepresentationTimestampTZ:
		result, err := utils.DecodeDateTime(input)
		if err != nil {
			return nil, err
		}

		return result.Format(time.RFC3339), err
	case *schema.TypeRepresentationJSON, *schema.TypeRepresentationGeography, *schema.TypeRepresentationGeometry:
		var result any
		err := json.Unmarshal([]byte(input), &result)

		return result, err
	default:
		return input, nil
	}
}
