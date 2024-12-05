package contenttype

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type ParameterItems []ParameterItem

// String implements fmt.Stringer interface
func (ssp ParameterItems) String() string {
	var results []string
	sortedPairs := append([]ParameterItem{}, ssp...)
	slices.SortFunc(sortedPairs, func(a, b ParameterItem) int {
		return strings.Compare(a.keys.String(), b.keys.String())
	})
	for _, item := range sortedPairs {
		if len(item.Values()) == 0 {
			continue
		}
		str := item.String()
		results = append(results, str)
	}

	return strings.Join(results, "&")
}

func (ssp *ParameterItems) Add(keys []Key, values []string) {
	index := ssp.FindIndex(keys)
	if index == -1 {
		*ssp = append(*ssp, NewParameterItem(keys, values))

		return
	}
	(*ssp)[index].AddValues(values)
}

func (ssp ParameterItems) FindDefault() *ParameterItem {
	item, _ := ssp.find([]Key{})
	if item != nil {
		return item
	}
	item, _ = ssp.find([]Key{})

	return item
}

func (ssp ParameterItems) Find(keys []Key) *ParameterItem {
	item, _ := ssp.find(keys)

	return item
}

func (ssp ParameterItems) FindIndex(keys []Key) int {
	_, i := ssp.find(keys)

	return i
}

func (ssp ParameterItems) find(keys []Key) (*ParameterItem, int) {
	for i, item := range ssp {
		if len(keys) != len(item.keys) {
			continue
		}
		if len(keys) == 0 {
			return &item, i
		}
		isEqual := false
		for j, value := range item.keys {
			isEqual = value == keys[j]
			if !isEqual {
				return nil, -1
			}
		}
		if isEqual {
			return &item, i
		}
	}

	return nil, -1
}

// Keys represent a key slice
type Keys []Key

// String implements fmt.Stringer interface
func (ks Keys) String() string {
	if len(ks) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, k := range ks {
		if k.index != nil {
			sb.WriteString(fmt.Sprintf("[%d]", *k.index))

			continue
		}
		if k.key != "" {
			if i > 0 {
				sb.WriteRune('.')
			}
			sb.WriteString(k.key)
		}
	}

	return sb.String()
}

// Key represents a key string or index
type Key struct {
	key   string
	index *int
}

// NewIndexKey creates an index key
func NewIndexKey(index int) Key {
	return Key{index: &index}
}

// NewKey creates a string key
func NewKey(key string) Key {
	return Key{key: key}
}

// IsEmpty checks if the key is empty
func (k Key) IsEmpty() bool {
	return k.key == "" && k.index == nil
}

// Key gets the string key
func (k Key) Key() string {
	return k.key
}

// Index gets the integer key
func (k Key) Index() *int {
	return k.index
}

// String implements fmt.Stringer interface
func (k Key) String() string {
	if k.index != nil {
		return strconv.Itoa(*k.index)
	}

	return k.key
}

// ParameterItem represents the key-value slice pair
type ParameterItem struct {
	keys   Keys
	values []string
}

// NewParameterItem creates a parameter value pair
func NewParameterItem(keys Keys, values []string) ParameterItem {
	return ParameterItem{
		keys:   keys,
		values: values,
	}
}

// String implements fmt.Stringer interface
func (ssp ParameterItem) String() string {
	key := ssp.keys.String()
	value := strings.Join(ssp.values, ",")
	if key == "" {
		return value
	}

	return fmt.Sprintf("%s=%s", key, value)
}

// Keys returns keys of the parameter item
func (ssp ParameterItem) Keys() Keys {
	return ssp.keys
}

func (ssp ParameterItem) Values() []string {
	return ssp.values
}

func (ssp *ParameterItem) Add(key Key, value string) {
	ssp.AddKey(key)
	ssp.AddValue(value)
}

func (ssp *ParameterItem) AddKey(key Key) {
	ssp.keys = append(ssp.keys, key)
}

func (ssp *ParameterItem) AddKeys(keys []Key) {
	ssp.keys = append(ssp.keys, keys...)
}

func (ssp *ParameterItem) AddValue(value string) {
	ssp.values = append(ssp.values, value)
}

func (ssp *ParameterItem) AddValues(values []string) {
	ssp.values = append(ssp.values, values...)
}

func (ssp *ParameterItem) PrependKey(key Key) {
	ssp.keys = append([]Key{key}, ssp.keys...)
}

func (ssp *ParameterItem) PrependKeys(keys []Key) {
	ssp.keys = append(keys, ssp.keys...)
}

func (ssp *ParameterItem) PrependValue(value string) {
	ssp.values = append([]string{value}, ssp.values...)
}

func (ssp *ParameterItem) PrependValues(values []string) {
	ssp.values = append(values, ssp.values...)
}
