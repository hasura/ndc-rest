package internal

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

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
			xmlTree := createXMLBlock(se)
			if err := c.evalXMLTree(xmlTree); err != nil {
				return nil, fmt.Errorf("failed to decode the xml result: %w", err)
			}

			result, err := c.evalXMLField(xmlTree, "", rest.ObjectField{
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

func (c *XMLDecoder) evalXMLField(block *xmlBlock, fieldName string, field rest.ObjectField, fieldPaths []string) (any, error) {
	rawType, err := field.Type.InterfaceT()
	if err != nil {
		return nil, err
	}

	switch t := rawType.(type) {
	case *schema.NullableType:
		return c.evalXMLField(block, fieldName, rest.ObjectField{
			ObjectField: schema.ObjectField{
				Type: t.UnderlyingType,
			},
			HTTP: field.HTTP,
		}, fieldPaths)
	case *schema.ArrayType:
		return c.evalArrayField(block, fieldName, field, t, fieldPaths)
	case *schema.NamedType:
		return c.evalNamedField(block, t, fieldPaths)
	default:
		return nil, err
	}
}

func (c *XMLDecoder) evalArrayField(block *xmlBlock, fieldName string, field rest.ObjectField, t *schema.ArrayType, fieldPaths []string) (any, error) {
	if block.Fields == nil {
		return nil, nil
	}
	if len(block.Fields) == 0 {
		return []any{}, nil
	}

	var elements []xmlBlock
	itemTokenName := fieldName
	wrapped := len(fieldPaths) == 0
	fieldItem := rest.ObjectField{
		ObjectField: schema.ObjectField{
			Type: t.ElementType,
		},
	}

	if field.HTTP != nil {
		wrapped = wrapped || (field.HTTP.XML != nil && field.HTTP.XML.Wrapped)
		if field.HTTP.Items != nil {
			fieldItem.HTTP = field.HTTP.Items
			if field.HTTP.Items.XML != nil && field.HTTP.Items.XML.Name != "" {
				itemTokenName = field.HTTP.Items.XML.Name
			}
		}
	}

	if wrapped {
		for _, elems := range block.Fields {
			if len(elems) > 0 {
				elements = elems

				break
			}
		}
	} else if elems, ok := block.Fields[itemTokenName]; ok {
		elements = elems
	}

	if len(elements) == 0 {
		return []any{}, nil
	}

	results := make([]any, len(elements))
	for i, elem := range elements {
		result, err := c.evalXMLField(&elem, itemTokenName, fieldItem, append(fieldPaths, strconv.Itoa(i)))
		if err != nil {
			return nil, err
		}
		results[i] = result
	}

	return results, nil
}

func (c *XMLDecoder) evalNamedField(block *xmlBlock, t *schema.NamedType, fieldPaths []string) (any, error) {
	if scalarType, ok := c.schema.ScalarTypes[t.Name]; ok {
		return c.decodeSimpleScalarValue(block.Data, scalarType, fieldPaths)
	}

	objectType, ok := c.schema.ObjectTypes[t.Name]
	if !ok {
		return nil, fmt.Errorf("%s: invalid response type", strings.Join(fieldPaths, "."))
	}

	result := map[string]any{}

	for _, attr := range block.Start.Attr {
		for key, objectField := range objectType.Fields {
			if objectField.HTTP == nil {
				continue
			}

			xmlKey := key
			if objectField.HTTP.XML != nil && objectField.HTTP.XML.Name != "" {
				xmlKey = objectField.HTTP.XML.Name
			}
			if attr.Name.Local != xmlKey {
				continue
			}

			attrValue, err := c.evalAttribute(objectField.Type, attr, append(fieldPaths, key))
			if err != nil {
				return nil, err
			}
			result[key] = attrValue

			break
		}
	}

	_, textFieldName, isLeaf := findXMLLeafObjectField(objectType)
	if isLeaf {
		textValue, err := c.decodeSimpleScalarValue(block.Data, c.schema.ScalarTypes[string(rest.ScalarString)], fieldPaths)
		if err != nil {
			return nil, err
		}

		result[textFieldName] = textValue

		return result, nil
	}

	for key, objectField := range objectType.Fields {
		if objectField.HTTP == nil {
			continue
		}
		xmlKey := key
		if objectField.HTTP.XML != nil {
			if objectField.HTTP.XML.Attribute {
				continue
			}

			xmlKey = getXMLName(objectField.HTTP.XML, key)
		}

		fieldElems, ok := block.Fields[xmlKey]
		if !ok || fieldElems == nil {
			continue
		}

		switch len(fieldElems) {
		case 0:
			result[key] = []any{}
		case 1:
			fieldResult, err := c.evalXMLField(&fieldElems[0], xmlKey, objectField, append(fieldPaths, key))
			if err != nil {
				return nil, err
			}

			result[key] = fieldResult
		default:
			fieldResult, err := c.evalXMLField(&xmlBlock{
				Start: fieldElems[0].Start,
				Fields: map[string][]xmlBlock{
					xmlKey: fieldElems,
				},
			}, xmlKey, objectField, append(fieldPaths, key))
			if err != nil {
				return nil, err
			}

			result[key] = fieldResult
		}
	}

	return result, nil
}

func (c *XMLDecoder) evalAttribute(schemaType schema.Type, attr xml.Attr, fieldPaths []string) (any, error) {
	rawType, err := schemaType.InterfaceT()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	switch t := rawType.(type) {
	case *schema.NullableType:
		return c.evalAttribute(t.UnderlyingType, attr, fieldPaths)
	case *schema.ArrayType:
		var result any
		if err := json.Unmarshal([]byte(attr.Value), &result); err != nil {
			return nil, fmt.Errorf("%s: failed to decode xml attribute, %w", strings.Join(fieldPaths, ","), err)
		}

		return result, nil
	case *schema.NamedType:
		if scalarType, ok := c.schema.ScalarTypes[t.Name]; ok {
			return c.decodeSimpleScalarValue(attr.Value, scalarType, fieldPaths)
		}

		var result any
		if err := json.Unmarshal([]byte(attr.Value), &result); err != nil {
			return nil, fmt.Errorf("%s: failed to decode xml attribute, %w", strings.Join(fieldPaths, ","), err)
		}

		return result, nil
	default:
		return nil, err
	}
}

func (c *XMLDecoder) decodeSimpleScalarValue(token string, scalarType schema.ScalarType, fieldPaths []string) (any, error) {
	respType, err := scalarType.Representation.InterfaceT()

	var result any = nil
	switch respType.(type) {
	case *schema.TypeRepresentationString:
		result = token
	case *schema.TypeRepresentationDate, *schema.TypeRepresentationTimestamp, *schema.TypeRepresentationTimestampTZ, *schema.TypeRepresentationUUID, *schema.TypeRepresentationEnum:
		if len(token) > 0 {
			result = token
		}
	case *schema.TypeRepresentationBytes:
		result = token
	case *schema.TypeRepresentationBoolean:
		if len(token) == 0 {
			break
		}

		result, err = strconv.ParseBool(token)
	case *schema.TypeRepresentationBigDecimal, *schema.TypeRepresentationBigInteger:
		if len(token) == 0 {
			break
		}

		result = token
	case *schema.TypeRepresentationInteger, *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64: //nolint:all
		if len(token) == 0 {
			break
		}

		result, err = strconv.ParseInt(token, 10, 64)
	case *schema.TypeRepresentationNumber, *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64: //nolint:all
		if len(token) == 0 {
			break
		}

		result, err = strconv.ParseFloat(token, 64)
	case *schema.TypeRepresentationGeography, *schema.TypeRepresentationGeometry, *schema.TypeRepresentationJSON:
		if len(token) == 0 {
			break
		}

		result = token
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	return result, nil
}

type xmlBlock struct {
	Start  xml.StartElement
	Data   string
	Fields map[string][]xmlBlock
}

func createXMLBlock(start xml.StartElement) *xmlBlock {
	return &xmlBlock{
		Start:  start,
		Fields: map[string][]xmlBlock{},
	}
}

func (c *XMLDecoder) evalXMLTree(block *xmlBlock) error {
L:
	for {
		nextToken, err := c.decoder.Token()
		if err != nil {
			return err
		}

		if nextToken == nil {
			return nil
		}

		switch tok := nextToken.(type) {
		case xml.StartElement:
			childBlock := createXMLBlock(tok)
			if err := c.evalXMLTree(childBlock); err != nil {
				return err
			}
			block.Fields[tok.Name.Local] = append(block.Fields[tok.Name.Local], *childBlock)
		case xml.CharData:
			block.Data = string(tok)
		case xml.EndElement:
			break L
		}
	}

	return nil
}
