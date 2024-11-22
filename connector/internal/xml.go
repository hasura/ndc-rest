package internal

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

// XMLEncoder implements a dynamic XML encoder from the HTTP schema.
type XMLEncoder struct {
	schema *rest.NDCHttpSchema
}

// NewXMLEncoder creates a new XML encoder.
func NewXMLEncoder(httpSchema *rest.NDCHttpSchema) *XMLEncoder {
	return &XMLEncoder{
		schema: httpSchema,
	}
}

// Encode marshals the body to xml bytes.
func (c *XMLEncoder) Encode(bodyInfo *rest.ArgumentInfo, bodyData any) ([]byte, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	err := c.evalXMLField(enc, "", rest.ObjectField{
		ObjectField: schema.ObjectField{
			Type: bodyInfo.Type,
		},
		HTTP: bodyInfo.HTTP.Schema,
	}, bodyData, []string{})

	if err != nil {
		return nil, err
	}

	if err := enc.Flush(); err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), buf.Bytes()...), nil
}

func (c *XMLEncoder) evalXMLField(enc *xml.Encoder, name string, field rest.ObjectField, value any, fieldPaths []string) error {
	rawType, err := field.Type.InterfaceT()
	var innerValue reflect.Value
	var notNull bool

	if value != nil {
		innerValue, notNull = utils.UnwrapPointerFromAnyToReflectValue(value)
	}

	switch t := rawType.(type) {
	case *schema.NullableType:
		if !notNull {
			return nil
		}

		return c.evalXMLField(enc, name, rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: t.UnderlyingType,
			},
			HTTP: field.HTTP,
		}, innerValue.Interface(), fieldPaths)
	case *schema.ArrayType:
		if !notNull {
			return fmt.Errorf("%s: expect an array, got null", strings.Join(fieldPaths, "."))
		}

		vi := innerValue.Interface()
		values, ok := vi.([]any)
		if !ok {
			return fmt.Errorf("%s: expect an array, got %v", strings.Join(fieldPaths, "."), vi)
		}

		var wrapped bool
		xmlName := name
		if field.HTTP.XML != nil {
			wrapped = field.HTTP.XML.Wrapped
			if field.HTTP.XML.Name != "" {
				xmlName = field.HTTP.XML.Name
			}
		}

		if wrapped {
			err := enc.EncodeToken(xml.StartElement{
				Name: xml.Name{Space: "", Local: xmlName},
			})
			if err != nil {
				return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
			}
		}

		for i, v := range values {
			err := c.evalXMLField(enc, name, rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: t.ElementType,
				},
				HTTP: field.HTTP.Items,
			}, v, append(fieldPaths, strconv.FormatInt(int64(i), 10)))

			if err != nil {
				return err
			}
		}

		if wrapped {
			err := enc.EncodeToken(xml.EndElement{
				Name: xml.Name{Space: "", Local: xmlName},
			})
			if err != nil {
				return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
			}
		}

		return nil
	case *schema.NamedType:
		if !notNull {
			return fmt.Errorf("%s: expect a non-null type, got null", strings.Join(fieldPaths, "."))
		}

		xmlName := t.Name
		if field.HTTP != nil && field.HTTP.XML != nil && field.HTTP.XML.Name != "" {
			xmlName = field.HTTP.XML.Name
		} else if name != "" {
			xmlName = name
		}

		if _, ok := c.schema.ScalarTypes[t.Name]; ok {
			if err := c.encodeSimpleScalar(enc, xmlName, reflect.ValueOf(value)); err != nil {
				return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
			}

			return nil
		}

		objectType, ok := c.schema.ObjectTypes[t.Name]
		if !ok {
			return fmt.Errorf("%s: invalid type %s", strings.Join(fieldPaths, "."), t.Name)
		}

		iv := innerValue.Interface()
		values, ok := iv.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected a map, got %s", strings.Join(fieldPaths, "."), innerValue.Kind())
		}

		if objectType.XML != nil && objectType.XML.Name != "" {
			xmlName = objectType.XML.Name
		}

		err := enc.EncodeToken(xml.StartElement{
			Name: xml.Name{Space: "", Local: xmlName},
		})
		if err != nil {
			return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
		}

		fieldKeys := utils.GetSortedKeys(objectType.Fields)
		for _, key := range fieldKeys {
			objectField := objectType.Fields[key]
			fieldValue := values[key]
			if err := c.evalXMLField(enc, key, objectField, fieldValue, append(fieldPaths, key)); err != nil {
				return err
			}
		}

		err = enc.EncodeToken(xml.EndElement{
			Name: xml.Name{Space: "", Local: xmlName},
		})
		if err != nil {
			return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
		}

		return nil
	default:
		return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}
}

func (c *XMLEncoder) encodeSimpleScalar(enc *xml.Encoder, name string, value reflect.Value) error {
	str, err := marshalSimpleScalar(value, value.Kind())
	if err != nil {
		return err
	}

	err = enc.EncodeToken(xml.StartElement{
		Name: xml.Name{Space: "", Local: name},
	})
	if err != nil {
		return err
	}

	if err := enc.EncodeToken(xml.CharData(str)); err != nil {
		return err
	}

	return enc.EncodeToken(xml.EndElement{
		Name: xml.Name{Space: "", Local: name},
	})
}

// XMLDecoder implements a dynamic XML decoder from the HTTP schema.
type XMLDecoder struct {
	schema  *rest.NDCHttpSchema
	decoder *xml.Decoder
}

// NewXMLDecoder creates a new XML encoder.
func NewXMLDecoder(httpSchema *rest.NDCHttpSchema) *XMLDecoder {
	return &XMLDecoder{
		schema: httpSchema,
	}
}

// Decode unmarshals xml bytes to a dynamic type.
func (c *XMLDecoder) Decode(r io.Reader, resultType schema.Type) (any, error) {
	c.decoder = xml.NewDecoder(r)

	for {
		token, err := c.decoder.Token()
		if err != nil {
			return nil, err
		}
		if token == nil {
			break
		}

		if se, ok := token.(xml.StartElement); ok {
			result, _, err := c.evalXMLField(se, "", rest.ObjectField{
				ObjectField: schema.ObjectField{
					Type: resultType,
				},
				HTTP: &rest.TypeSchema{},
			}, []string{})
			if err != nil {
				return nil, fmt.Errorf("failed to decode the xml result: %w", err)
			}

			return result, nil
		}
	}

	return nil, nil
}

func (c *XMLDecoder) evalXMLField(token xml.StartElement, fieldName string, field rest.ObjectField, fieldPaths []string) (any, xml.Token, error) {
	rawType, err := field.Type.InterfaceT()
	if err != nil {
		return nil, nil, err
	}

	switch t := rawType.(type) {
	case *schema.NullableType:
		return c.evalXMLField(token, fieldName, rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: t.UnderlyingType,
			},
			HTTP: field.HTTP,
		}, fieldPaths)
	case *schema.ArrayType:
		return c.evalXMLArrayField(token, fieldName, field, t, fieldPaths)
	case *schema.NamedType:
		return c.evalXMLNamedField(token, fieldName, t, fieldPaths)
	default:
		return nil, nil, err
	}
}

func (c *XMLDecoder) evalXMLNamedField(token xml.StartElement, fieldName string, t *schema.NamedType, fieldPaths []string) (any, xml.Token, error) {
	if scalarType, ok := c.schema.ScalarTypes[t.Name]; ok {
		nextToken, err := c.decoder.Token()
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
		}

		if nextToken == nil {
			return nil, nil, nil
		}

		switch nt := nextToken.(type) {
		case xml.CharData:
			return c.decodeSimpleScalar(nt, scalarType, fieldPaths)
		case xml.EndElement:
			return c.decodeSimpleScalar(make(xml.CharData, 0), scalarType, fieldPaths)
		default:
			return nil, nil, fmt.Errorf("invalid xml token: %s", nextToken)
		}
	}

	objectType, ok := c.schema.ObjectTypes[t.Name]
	if !ok {
		return nil, nil, fmt.Errorf("%s: invalid response type", strings.Join(fieldPaths, "."))
	}

	// the root field may have a different tag name, we can skip the validation
	if len(fieldPaths) > 0 && (fieldName != "" && token.Name.Local != fieldName) && (objectType.XML == nil || objectType.XML.Name != token.Name.Local) {
		return nil, nil, fmt.Errorf("%s:invalid token, expected: %s, got: %+v", strings.Join(fieldPaths, "."), token.Name.Local, objectType.XML)
	}
	nextToken, err := c.decoder.Token()
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	if nextToken == nil {
		return nil, nil, nil
	}

	result := make(map[string]any)
L:
	for nextToken != nil {
		switch nt := nextToken.(type) {
		case xml.StartElement:
			var hasField bool
			for key, objectField := range objectType.Fields {
				if objectField.HTTP == nil {
					continue
				}
				xmlKey := key
				if objectField.HTTP.XML != nil && objectField.HTTP.XML.Name != "" {
					xmlKey = objectField.HTTP.XML.Name
				}

				if nt.Name.Local != xmlKey {
					continue
				}

				fieldResult, nt, err := c.evalXMLField(nt, key, objectField, append(fieldPaths, key))
				if err != nil {
					return nil, nil, err
				}

				nextToken = nt
				result[key] = fieldResult
				hasField = true

				break
			}

			if !hasField {
				nextToken, err = c.skipElement(nt.Name.Local)
				if err != nil {
					return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
				}
			}
		case xml.EndElement:
			nextToken, err = c.decoder.Token()
			if err != nil && !errors.Is(err, io.EOF) {
				return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
			}

			if nt.Name.Local == token.Name.Local {
				break L
			}
		default:
			nextToken, err = c.skipElement("")
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
			}
		}
	}

	return result, nextToken, nil
}

func (c *XMLDecoder) evalXMLArrayField(token xml.StartElement, fieldName string, field rest.ObjectField, t *schema.ArrayType, fieldPaths []string) (any, xml.Token, error) {
	var wrapped bool
	var nextToken xml.Token = token
	var itemTokenName string
	var err error

	fieldItem := rest.ObjectField{
		ObjectField: schema.ObjectField{
			Type: t.ElementType,
		},
	}

	if field.HTTP != nil {
		wrapped = field.HTTP.XML != nil && field.HTTP.XML.Wrapped
		if field.HTTP.Items != nil {
			fieldItem.HTTP = field.HTTP.Items
			if field.HTTP.Items.XML != nil && field.HTTP.Items.XML.Name != "" {
				itemTokenName = field.HTTP.Items.XML.Name
			}
		}
	}

	if wrapped {
		switch nt := nextToken.(type) {
		case xml.StartElement:
			nextToken, err = c.decoder.Token()
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
			}
			if nextToken == nil {
				return nil, nil, fmt.Errorf("%s: expected close tag %s, got nil", strings.Join(fieldPaths, ""), token.Name.Local)
			}
			if itemTokenName == "" {
				// find the tag name from object type
				namedType := schema.GetUnderlyingNamedType(t.ElementType)
				if namedType == nil {
					return nil, nil, fmt.Errorf("%s: the element type is null", strings.Join(fieldPaths, ""))
				}

				if objectType, ok := c.schema.ObjectTypes[namedType.Name]; ok && objectType.XML != nil && objectType.XML.Name != "" {
					itemTokenName = objectType.XML.Name
				}
			}
		case xml.EndElement:
			if token.Name.Local != nt.Name.Local {
				return nil, nil, fmt.Errorf("%s: expected close tag %s, got nil", strings.Join(fieldPaths, ""), token.Name.Local)
			}

			nextToken, err := c.decoder.Token()
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, ""), err)
			}

			return []any{}, nextToken, nil
		}
	} else {
		itemTokenName = token.Name.Local
	}

	var i int64
	results := make([]any, 0)
LA:
	for {
		if nextToken == nil {
			return results, nil, nil
		}

		switch nt := nextToken.(type) {
		case xml.StartElement:
			if itemTokenName != nt.Name.Local {
				if wrapped {
					return nil, nil, fmt.Errorf("%s: expected start element tag of the array item, expected %s, got %s", strings.Join(fieldPaths, ""), itemTokenName, nt.Name.Local)
				}

				break LA
			}

			item, tok, err := c.evalXMLField(nt, fieldName, fieldItem, append(fieldPaths, strconv.FormatInt(i, 10)))
			if err != nil {
				return nil, nil, err
			}

			results = append(results, item)
			nextToken = tok
			i++
		case xml.EndElement:
			break LA
		default:
			nextToken, err = c.decoder.Token()
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
			}
		}
	}

	if !wrapped {
		return results, nextToken, nil
	}

	switch nt := nextToken.(type) {
	case xml.EndElement:
		if nt.Name.Local != token.Name.Local {
			return nil, nil, fmt.Errorf("%s: expected end tag %s, got %s", strings.Join(fieldPaths, ""), token.Name.Local, nt.Name.Local)
		}

		nextToken, err := c.decoder.Token()
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
		}

		return results, nextToken, nil
	default:
		return nil, nil, fmt.Errorf("%s: unexpected end tag, got: %v", strings.Join(fieldPaths, "."), nextToken)
	}
}

func (c *XMLDecoder) decodeSimpleScalar(token xml.CharData, scalarType schema.ScalarType, fieldPaths []string) (any, xml.Token, error) {
	respType, err := scalarType.Representation.InterfaceT()

	var result any = nil
	switch respType.(type) {
	case *schema.TypeRepresentationString:
		result = string(token)
	case *schema.TypeRepresentationDate, *schema.TypeRepresentationTimestamp, *schema.TypeRepresentationTimestampTZ, *schema.TypeRepresentationUUID, *schema.TypeRepresentationEnum:
		if len(token) > 0 {
			result = string(token)
		}
	case *schema.TypeRepresentationBytes:
		result = token
	case *schema.TypeRepresentationBoolean:
		if len(token) == 0 {
			break
		}

		result, err = strconv.ParseBool(string(token))
	case *schema.TypeRepresentationBigDecimal, *schema.TypeRepresentationBigInteger:
		if len(token) == 0 {
			break
		}

		result = string(token)
	case *schema.TypeRepresentationInteger, *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64: //nolint:all
		if len(token) == 0 {
			break
		}

		result, err = strconv.ParseInt(string(token), 10, 64)
	case *schema.TypeRepresentationNumber, *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64: //nolint:all
		if len(token) == 0 {
			break
		}

		result, err = strconv.ParseFloat(string(token), 64)
	case *schema.TypeRepresentationGeography, *schema.TypeRepresentationGeometry, *schema.TypeRepresentationJSON:
		if len(token) == 0 {
			break
		}

		result = string(token)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	nextToken, err := c.skipElement("")
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	return result, nextToken, nil
}

func (c *XMLDecoder) skipElement(tagName string) (xml.Token, error) {
	for {
		tok, err := c.decoder.Token()
		if err != nil {
			return nil, err
		}

		if tok == nil {
			return nil, nil
		}

		t, ok := tok.(xml.EndElement)
		if !ok {
			continue
		}

		if tagName == "" || t.Name.Local == tagName {
			return c.decoder.Token()
		}
	}
}
