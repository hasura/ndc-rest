package internal

import (
	"fmt"
	"strings"
)

type StringSlicePairs []StringSlicePair

// String implements fmt.Stringer interface
func (ssp StringSlicePairs) String() string {
	results := make([]string, len(ssp))
	for _, item := range ssp {
		str := item.String()
		if str == "" {
			continue
		}
		results = append(results, str)
	}
	return strings.Join(results, "&")
}

func (ssp *StringSlicePairs) Add(keys []string, values []string) {
	index := ssp.FindIndex(keys)
	if index == -1 {
		*ssp = append(*ssp, NewStringSlicePair(keys, values))
		return
	}
	(*ssp)[index].AddValues(values)
}

func (ssp StringSlicePairs) FindDefault() *StringSlicePair {
	item, _ := ssp.find([]string{})
	if item != nil {
		return item
	}
	item, _ = ssp.find([]string{""})
	return item
}

func (ssp StringSlicePairs) Find(keys []string) *StringSlicePair {
	item, _ := ssp.find(keys)
	return item
}

func (ssp StringSlicePairs) FindIndex(keys []string) int {
	_, i := ssp.find(keys)
	return i
}

func (ssp StringSlicePairs) find(keys []string) (*StringSlicePair, int) {
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
	}
	return nil, -1
}

type StringSlicePair struct {
	keys   []string
	values []string
}

// NewStringSlicePair creates a string slice pair
func NewStringSlicePair(keys []string, values []string) StringSlicePair {
	return StringSlicePair{
		keys:   keys,
		values: values,
	}
}

// String implements fmt.Stringer interface
func (ssp StringSlicePair) String() string {
	key := strings.Join(ssp.keys, "")
	value := strings.Join(ssp.values, ",")
	if key == "" {
		return value
	}
	return fmt.Sprintf("%s=%s", key, value)
}

func (ssp StringSlicePair) Keys() []string {
	return ssp.keys
}

func (ssp StringSlicePair) Values() []string {
	return ssp.values
}

func (ssp *StringSlicePair) Add(key string, value string) {
	ssp.AddKey(key)
	ssp.AddValue(value)
}

func (ssp *StringSlicePair) AddKey(key string) {
	ssp.keys = append(ssp.keys, key)
}

func (ssp *StringSlicePair) AddKeys(keys []string) {
	ssp.keys = append(ssp.keys, keys...)
}

func (ssp *StringSlicePair) AddValue(value string) {
	ssp.values = append(ssp.values, value)
}

func (ssp *StringSlicePair) AddValues(values []string) {
	ssp.values = append(ssp.values, values...)
}

func (ssp *StringSlicePair) PrependKey(key string) {
	ssp.keys = append([]string{key}, ssp.keys...)
}

func (ssp *StringSlicePair) PrependKeys(keys []string) {
	ssp.keys = append(keys, ssp.keys...)
}

func (ssp *StringSlicePair) PrependValue(value string) {
	ssp.values = append([]string{value}, ssp.values...)
}

func (ssp *StringSlicePair) PrependValues(values []string) {
	ssp.values = append(values, ssp.values...)
}
