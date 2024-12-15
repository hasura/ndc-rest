package contenttype

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
			if err := evalXMLTree(c.decoder, xmlTree); err != nil {
				return nil, fmt.Errorf("failed to decode the xml result: %w", err)
			}

			if c.schema == nil {
				return decodeArbitraryXMLBlock(xmlTree), nil
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

func (c *XMLDecoder) getArrayItemObjectField(field rest.ObjectField, t *schema.ArrayType) rest.ObjectField {
	fieldItem := rest.ObjectField{
		ObjectField: schema.ObjectField{
			Type: t.ElementType,
		},
	}

	if field.HTTP != nil && field.HTTP.Items != nil {
		fieldItem.HTTP = field.HTTP.Items
	}

	return fieldItem
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
	fieldItem := c.getArrayItemObjectField(field, t)

	if field.HTTP != nil {
		wrapped = wrapped || (field.HTTP.XML != nil && field.HTTP.XML.Wrapped)
		if field.HTTP.Items != nil && field.HTTP.Items.XML != nil && field.HTTP.Items.XML.Name != "" {
			itemTokenName = field.HTTP.Items.XML.Name
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

	return c.evalArrayElements(elements, itemTokenName, fieldItem, fieldPaths)
}

func (c *XMLDecoder) evalArrayElements(elements []xmlBlock, itemTokenName string, fieldItem rest.ObjectField, fieldPaths []string) ([]any, error) {
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
		return c.decodeSimpleScalarValue(block, scalarType, fieldPaths)
	}

	objectType, ok := c.schema.ObjectTypes[t.Name]
	if !ok {
		return nil, fmt.Errorf("%s: invalid response type", strings.Join(fieldPaths, "."))
	}

	result := map[string]any{}

	for _, attr := range block.Start.Attr {
		for key, objectField := range objectType.Fields {
			if objectField.HTTP == nil || objectField.HTTP.XML == nil || !objectField.HTTP.XML.Attribute {
				continue
			}

			xmlKey := key
			if objectField.HTTP.XML.Name != "" {
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
		textValue, err := c.decodeSimpleScalarValue(block, c.schema.ScalarTypes[string(rest.ScalarString)], fieldPaths)
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
			propPaths := append(fieldPaths, key)
			if objectField.HTTP.XML != nil && objectField.HTTP.XML.Wrapped {
				// this can be a wrapped array
				fieldResult, err := c.evalXMLField(&fieldElems[0], xmlKey, objectField, propPaths)
				if err != nil {
					return nil, err
				}

				result[key] = fieldResult

				continue
			}

			at, nt, err := getArrayOrNamedType(objectField.Type)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", strings.Join(propPaths, "."), err)
			}

			if at != nil {
				fieldItem := c.getArrayItemObjectField(objectField, at)
				fieldResult, err := c.evalArrayElements(fieldElems, xmlKey, fieldItem, propPaths)
				if err != nil {
					return nil, err
				}

				result[key] = fieldResult
			} else if nt != nil {
				fieldResult, err := c.evalNamedField(&fieldElems[0], nt, propPaths)
				if err != nil {
					return nil, err
				}

				result[key] = fieldResult
			}
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
			return c.decodeSimpleScalarValue(&xmlBlock{
				Data: attr.Value,
			}, scalarType, fieldPaths)
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

func (c *XMLDecoder) decodeSimpleScalarValue(block *xmlBlock, scalarType schema.ScalarType, fieldPaths []string) (any, error) {
	respType, err := scalarType.Representation.InterfaceT()

	var result any = nil
	switch respType.(type) {
	case *schema.TypeRepresentationString:
		result = block.Data
	case *schema.TypeRepresentationDate, *schema.TypeRepresentationTimestamp, *schema.TypeRepresentationTimestampTZ, *schema.TypeRepresentationUUID, *schema.TypeRepresentationEnum:
		if len(block.Data) > 0 {
			result = block.Data
		}
	case *schema.TypeRepresentationBytes:
		result = block.Data
	case *schema.TypeRepresentationBoolean:
		if len(block.Data) == 0 {
			break
		}

		result, err = strconv.ParseBool(block.Data)
	case *schema.TypeRepresentationBigDecimal, *schema.TypeRepresentationBigInteger:
		if len(block.Data) == 0 {
			break
		}

		result = block.Data
	case *schema.TypeRepresentationInteger, *schema.TypeRepresentationInt8, *schema.TypeRepresentationInt16, *schema.TypeRepresentationInt32, *schema.TypeRepresentationInt64: //nolint:all
		if len(block.Data) == 0 {
			break
		}

		result, err = strconv.ParseInt(block.Data, 10, 64)
	case *schema.TypeRepresentationNumber, *schema.TypeRepresentationFloat32, *schema.TypeRepresentationFloat64: //nolint:all
		if len(block.Data) == 0 {
			break
		}

		result, err = strconv.ParseFloat(block.Data, 64)
	case *schema.TypeRepresentationGeography, *schema.TypeRepresentationGeometry, *schema.TypeRepresentationJSON:
		result = decodeArbitraryXMLBlock(block)
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	return result, nil
}

func decodeArbitraryXMLBlock(block *xmlBlock) any {
	if len(block.Start.Attr) == 0 && len(block.Fields) == 0 {
		return block.Data
	}

	result := make(map[string]any)
	if len(block.Start.Attr) > 0 {
		attributes := make(map[string]string)
		for _, attr := range block.Start.Attr {
			attributes[attr.Name.Local] = attr.Value
		}
		result["attributes"] = attributes
	}

	if len(block.Fields) == 0 {
		result["content"] = block.Data

		return result
	}

	for key, field := range block.Fields {
		switch len(field) {
		case 0:
		case 1:
			// limitation: we can't know if the array is wrapped
			result[key] = decodeArbitraryXMLBlock(&field[0])
		default:
			items := make([]any, len(field))
			for i, f := range field {
				items[i] = decodeArbitraryXMLBlock(&f)
			}
			result[key] = items
		}
	}

	return result
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

func evalXMLTree(decoder *xml.Decoder, block *xmlBlock) error {
L:
	for {
		nextToken, err := decoder.Token()
		if err != nil {
			return err
		}

		if nextToken == nil {
			return nil
		}

		switch tok := nextToken.(type) {
		case xml.StartElement:
			childBlock := createXMLBlock(tok)
			if err := evalXMLTree(decoder, childBlock); err != nil {
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

// DecodeArbitraryXML decodes an arbitrary XML from a reader stream.
func DecodeArbitraryXML(r io.Reader) (any, error) {
	decoder := xml.NewDecoder(r)

	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if token == nil {
			break
		}

		if se, ok := token.(xml.StartElement); ok {
			xmlTree := createXMLBlock(se)
			if err := evalXMLTree(decoder, xmlTree); err != nil {
				return nil, fmt.Errorf("failed to decode the xml result: %w", err)
			}

			result := decodeArbitraryXMLBlock(xmlTree)

			return result, nil
		}
	}

	return nil, nil
}

func findXMLLeafObjectField(objectType rest.ObjectType) (*rest.ObjectField, string, bool) {
	var f *rest.ObjectField
	var fieldName string
	for key, field := range objectType.Fields {
		if field.HTTP == nil || field.HTTP.XML == nil {
			return nil, "", false
		}
		if field.HTTP.XML.Text {
			f = &field
			fieldName = key
		} else if !field.HTTP.XML.Attribute {
			return nil, "", false
		}
	}

	return f, fieldName, true
}

func getTypeSchemaXMLName(typeSchema *rest.TypeSchema, defaultName string) string {
	if typeSchema != nil {
		return getXMLName(typeSchema.XML, defaultName)
	}

	return defaultName
}

func getXMLName(xmlSchema *rest.XMLSchema, defaultName string) string {
	if xmlSchema != nil {
		if xmlSchema.Name != "" {
			return xmlSchema.GetFullName()
		}

		if xmlSchema.Prefix != "" {
			return xmlSchema.Prefix + ":" + defaultName
		}
	}

	return defaultName
}

func getArrayOrNamedType(schemaType schema.Type) (*schema.ArrayType, *schema.NamedType, error) {
	rawType, err := schemaType.InterfaceT()
	if err != nil {
		return nil, nil, err
	}

	switch t := rawType.(type) {
	case *schema.NullableType:
		return getArrayOrNamedType(t.UnderlyingType)
	case *schema.ArrayType:
		return t, nil, nil
	case *schema.NamedType:
		return nil, t, nil
	default:
		return nil, nil, nil
	}
}
