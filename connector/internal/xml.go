package internal

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

func (c *RequestBuilder) createXMLBody(bodyInfo *rest.ArgumentInfo, bodyData any) ([]byte, error) {
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

	return buf.Bytes(), nil
}

func (c *RequestBuilder) evalXMLField(enc *xml.Encoder, name string, field rest.ObjectField, value any, fieldPaths []string) error {
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

		xmlName := name
		var wrapped bool
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

		if _, ok := c.Schema.ScalarTypes[t.Name]; ok {
			if err := xmlEncodeSimpleScalar(enc, xmlName, reflect.ValueOf(value)); err != nil {
				return fmt.Errorf("%s: %w", strings.Join(fieldPaths, "."), err)
			}

			return nil
		}

		objectType, ok := c.Schema.ObjectTypes[t.Name]
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

func xmlEncodeSimpleScalar(enc *xml.Encoder, name string, value reflect.Value) error {
	str, err := marshalSimple(value)
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
