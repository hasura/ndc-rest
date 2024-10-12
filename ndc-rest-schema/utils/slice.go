package utils

import (
	"cmp"
	"slices"
)

// SliceUnorderedEqual compares if both slices are equal with unordered positions
func SliceUnorderedEqual[T cmp.Ordered](a []T, b []T) bool {
	sortedA := slices.Clone(a)
	slices.Sort(sortedA)
	sortedB := slices.Clone(b)
	slices.Sort(sortedB)

	return slices.Equal(sortedA, sortedB)
}
