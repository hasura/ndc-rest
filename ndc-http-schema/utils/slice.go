package utils

import (
	"cmp"
	"slices"

	"github.com/hasura/ndc-sdk-go/utils"
)

// SliceUnorderedEqual compares if both slices are equal with unordered positions
func SliceUnorderedEqual[T cmp.Ordered](a []T, b []T) bool {
	sortedA := slices.Clone(a)
	slices.Sort(sortedA)
	sortedB := slices.Clone(b)
	slices.Sort(sortedB)

	return slices.Equal(sortedA, sortedB)
}

// SliceUnique gets unique elements of the input slice.
func SliceUnique[T cmp.Ordered](input []T) []T {
	if len(input) == 0 {
		return []T{}
	}

	valueMap := make(map[T]bool)
	for _, elem := range input {
		valueMap[elem] = true
	}

	return utils.GetSortedKeys(valueMap)
}
