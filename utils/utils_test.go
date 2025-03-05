package utils

import (
	"reflect"
	"testing"
)

func TestPartition(t *testing.T) {
	even, odd := Partition([]int{1, 2, 3, 4, 5, 8}, func(i int) bool { return (i % 2) == 0 })
	// if !reflect.DeepEqual(odd, []int{1, 3, 5}) {
	if !reflect.DeepEqual(odd, []int{1, 3, 5}) {
		t.Errorf("%v should be odd numbers", odd)
	}
	if !reflect.DeepEqual(even, []int{2, 4, 8}) {
		t.Errorf("%v should be even numbers", even)
	}

	_, none := Partition([]int{1, 2, 3, 4, 5, 8}, func(i int) bool { return true })
	if len(none) != 0 {
		t.Errorf("%v should be empty", none)
	}

}

func TestFields(t *testing.T) {
	tests := []struct {
		name string
		args string
		want []string
	}{
		{"empty", "", nil},
		{"simple with trim", " a  b c", []string{"a", "b", "c"}},
		{"quoted with trim", ` a  " b c"  d`, []string{"a", " b c", "d"}},
		{"single quote", ` a  'b c' d`, []string{"a", "'b", "c'", "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Fields(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Fields() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
