package contenttype

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
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

// Encode marshals the arbitrary body to xml bytes.
func (c *XMLEncoder) EncodeArbitrary(bodyData any) ([]byte, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	err := c.encodeSimpleScalar(enc, "xml", reflect.ValueOf(bodyData), nil, []string{})
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

		xmlName := getTypeSchemaXMLName(field.HTTP, name)
		var attributes []xml.Attr
		if field.HTTP != nil && field.HTTP.XML != nil && field.HTTP.XML.Namespace != "" {
			attributes = append(attributes, field.HTTP.XML.GetNamespaceAttribute())
		}

		if _, ok := c.schema.ScalarTypes[t.Name]; ok {
			if err := c.encodeSimpleScalar(enc, xmlName, reflect.ValueOf(value), attributes, fieldPaths); err != nil {
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

		attributes, fieldKeys, err := c.evalAttributes(objectType, utils.GetSortedKeys(objectType.Fields), values, fieldPaths)
		if err != nil {
			return err
		}

		xmlName = getXMLName(objectType.XML, name)
		if objectType.XML != nil && objectType.XML.Namespace != "" {
			attributes = append(attributes, objectType.XML.GetNamespaceAttribute())
		}

		err = enc.EncodeToken(xml.StartElement{
			Name: xml.Name{Space: "", Local: xmlName},
			Attr: attributes,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
		}

		if len(fieldKeys) == 1 && objectType.Fields[fieldKeys[0]].HTTP != nil && objectType.Fields[fieldKeys[0]].HTTP.XML != nil && objectType.Fields[fieldKeys[0]].HTTP.XML.Text {
			objectField := objectType.Fields[fieldKeys[0]]
			fieldValue, ok := values[fieldKeys[0]]
			if ok && fieldValue != nil {
				textValue, err := c.encodeXMLText(objectField.Type, reflect.ValueOf(fieldValue), fieldPaths)
				if err != nil {
					return err
				}

				if textValue != nil {
					err = enc.EncodeToken(xml.CharData(*textValue))
					if err != nil {
						return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
					}
				}
			}
		} else {
			for _, key := range fieldKeys {
				objectField := objectType.Fields[key]
				fieldValue := values[key]
				if err := c.evalXMLField(enc, key, objectField, fieldValue, append(fieldPaths, key)); err != nil {
					return err
				}
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

func (c *XMLEncoder) evalAttributes(objectType rest.ObjectType, keys []string, values map[string]any, fieldPaths []string) ([]xml.Attr, []string, error) {
	var attrs []xml.Attr
	remainKeys := make([]string, 0)
	for _, key := range keys {
		objectField := objectType.Fields[key]
		isNullXML := objectField.HTTP == nil || objectField.HTTP.XML == nil
		if isNullXML || !objectField.HTTP.XML.Attribute {
			remainKeys = append(remainKeys, key)

			continue
		}

		value, ok := values[key]
		if !ok || value == nil {
			continue
		}

		// the attribute value is usually a primitive scalar,
		// otherwise just encode the value as json string
		str, err := c.encodeXMLText(objectField.Type, reflect.ValueOf(value), append(fieldPaths, key))
		if err != nil {
			return nil, nil, err
		}

		if str == nil {
			continue
		}

		attrs = append(attrs, xml.Attr{
			Name:  xml.Name{Local: getTypeSchemaXMLName(objectField.HTTP, key)},
			Value: *str,
		})
	}

	return attrs, remainKeys, nil
}

func (c *XMLEncoder) encodeXMLText(schemaType schema.Type, value reflect.Value, fieldPaths []string) (*string, error) {
	rawType, err := schemaType.InterfaceT()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	innerValue, notNull := utils.UnwrapPointerFromReflectValue(value)

	switch t := rawType.(type) {
	case *schema.NullableType:
		if !notNull {
			return nil, nil
		}

		return c.encodeXMLText(t.UnderlyingType, value, fieldPaths)
	case *schema.ArrayType:
		if !notNull {
			return nil, fmt.Errorf("%s: field it required", strings.Join(fieldPaths, "."))
		}

		resultBytes, err := json.Marshal(innerValue.Interface())
		if err != nil {
			return nil, fmt.Errorf("%s: failed to encode xml attribute, %w", strings.Join(fieldPaths, "."), err)
		}

		result := string(resultBytes)

		return &result, nil
	case *schema.NamedType:
		if !notNull {
			return nil, fmt.Errorf("%s, field it required", strings.Join(fieldPaths, "."))
		}

		if _, ok := c.schema.ScalarTypes[t.Name]; ok {
			str, err := StringifySimpleScalar(value, value.Kind())
			if err != nil {
				return nil, err
			}

			return &str, nil
		}

		resultBytes, err := json.Marshal(innerValue.Interface())
		if err != nil {
			return nil, fmt.Errorf("%s: failed to encode xml attribute, %w", strings.Join(fieldPaths, "."), err)
		}

		result := string(resultBytes)

		return &result, nil
	default:
		return nil, fmt.Errorf("%s: failed to encode xml attribute, unsupported schema type", strings.Join(fieldPaths, "."))
	}
}

func (c *XMLEncoder) encodeSimpleScalar(enc *xml.Encoder, name string, reflectValue reflect.Value, attributes []xml.Attr, fieldPaths []string) error {
	reflectValue, ok := utils.UnwrapPointerFromReflectValue(reflectValue)
	if !ok {
		return nil
	}

	kind := reflectValue.Kind()
	switch kind {
	case reflect.Slice, reflect.Array:
		if len(fieldPaths) == 0 {
			err := enc.EncodeToken(xml.StartElement{
				Name: xml.Name{Local: name},
			})
			if err != nil {
				return err
			}
		}

		for i := 0; i < reflectValue.Len(); i++ {
			item := reflectValue.Index(i)
			if err := c.encodeSimpleScalar(enc, name, item, attributes, append(fieldPaths, strconv.Itoa(i))); err != nil {
				return err
			}
		}

		if len(fieldPaths) == 0 {
			err := enc.EncodeToken(xml.EndElement{
				Name: xml.Name{Local: name},
			})
			if err != nil {
				return err
			}
		}

		return nil
	case reflect.Map:
		ri := reflectValue.Interface()
		valueMap, ok := ri.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected map[string]any, got: %v", strings.Join(fieldPaths, "."), ri)
		}

		return c.encodeScalarMap(enc, name, valueMap, attributes, fieldPaths)
	case reflect.Interface:
		ri := reflectValue.Interface()
		valueMap, ok := ri.(map[string]any)
		if ok {
			return c.encodeScalarMap(enc, name, valueMap, attributes, fieldPaths)
		}

		return c.encodeScalarString(enc, name, reflectValue, kind, attributes, fieldPaths)
	default:
		return c.encodeScalarString(enc, name, reflectValue, kind, attributes, fieldPaths)
	}
}

func (c *XMLEncoder) encodeScalarMap(enc *xml.Encoder, name string, valueMap map[string]any, attributes []xml.Attr, fieldPaths []string) error {
	err := enc.EncodeToken(xml.StartElement{
		Name: xml.Name{Local: name},
		Attr: attributes,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	keys := utils.GetSortedKeys(valueMap)
	for _, key := range keys {
		item := valueMap[key]
		if err := c.encodeSimpleScalar(enc, key, reflect.ValueOf(item), nil, append(fieldPaths, key)); err != nil {
			return err
		}
	}

	err = enc.EncodeToken(xml.EndElement{
		Name: xml.Name{Local: name},
	})
	if err != nil {
		return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	return nil
}

func (c *XMLEncoder) encodeScalarString(enc *xml.Encoder, name string, reflectValue reflect.Value, kind reflect.Kind, attributes []xml.Attr, fieldPaths []string) error {
	str, err := StringifySimpleScalar(reflectValue, kind)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	err = enc.EncodeToken(xml.StartElement{
		Name: xml.Name{Local: name},
		Attr: attributes,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	if err := enc.EncodeToken(xml.CharData(str)); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	err = enc.EncodeToken(xml.EndElement{
		Name: xml.Name{Local: name},
	})
	if err != nil {
		return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
	}

	return nil
}
