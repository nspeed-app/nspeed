// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package str

import (
	"reflect"
	"testing"
)

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
