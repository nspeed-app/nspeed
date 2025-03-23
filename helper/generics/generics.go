// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package generics

// a collection of generic functions not yet in std lib

// Map applies a function f to each element of slice
// todo: they're going with adapters on iterators ?! meh -> https://github.com/golang/go/issues/61898
func Map[S ~[]U, U any, V any](a S, f func(U) V) []V {
	var lv = make([]V, len(a))
	for i, u := range a {
		lv[i] = f(u)
	}
	return lv
}

// Partition separates the elements of an array into 2 arrays based a predicate test function
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

// Prt returns the address of its argument.
// This, for instance, allows to return the address of the result of a function
func Ptr[T any](x T) *T {
	return &x
}

// OrDefault returns a or b if a is zero
func OrDefault[T comparable](a, b T) T {
	var zero T // I prefer this to "a == *new(T)"
	if a == zero {
		return b
	}
	return a
}
