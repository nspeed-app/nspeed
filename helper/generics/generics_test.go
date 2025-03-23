// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package generics

import (
	"reflect"
	"testing"
)

func TestPartition(t *testing.T) {
	even, odd := Partition([]int{1, 2, 6, 4, 5, 8}, func(i int) bool { return (i % 2) == 0 })
	// if !reflect.DeepEqual(odd, []int{1, 3, 5}) {
	if !reflect.DeepEqual(odd, []int{1, 5}) {
		t.Errorf("%v should be odd numbers", odd)
	}
	if !reflect.DeepEqual(even, []int{2, 6, 4, 8}) {
		t.Errorf("%v should be even numbers", even)
	}

	_, none := Partition([]int{1, 2, 3, 4, 5, 8}, func(i int) bool { return true })
	if len(none) != 0 {
		t.Errorf("%v should be empty", none)
	}

}
