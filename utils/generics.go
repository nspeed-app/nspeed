package utils

import "golang.org/x/exp/constraints"

// a collection of generic functions noy yet in std lib

// Min returns the smaller of x or y.
func Min[T constraints.Ordered](x, y T) T {
	if x < y {
		return x
	}
	return y
}

// Max returns the larger of x or y.
func Max[T constraints.Ordered](x, y T) T {
	if x > y {
		return x
	}
	return y
}

// Map applies function f to each element of array a
func Map[U any, V any](a []U, f func(U) V) []V {
	var lv = make([]V, len(a))
	for i, u := range a {
		lv[i] = f(u)
	}
	return lv
}

// Partition separates the elements of array into 2 arrays based on the predicate function test
func Partition[T any](array []T, test func(T) bool) (matched []T, notmatched []T) {
	for _, t := range array {
		if test(t) {
			matched = append(matched, t)
		} else {
			notmatched = append(notmatched, t)
		}
	}
	return
}

// IndexOf searches for an element in a slice and returns its index or -1 if not found
// todo: find and use the 'canonical' name for this function
// todo: watch for future equivalence in std lib
func IndexOf[T comparable](ss []T, s T) (idx int) {
	for idx, v := range ss {
		if v == s {
			return idx
		}
	}
	return -1
}
