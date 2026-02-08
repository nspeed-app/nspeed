// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package web

import "testing"

var t2 = `import "node.js";
	console.log(test);`

func Test_removeImport(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{"T1", "import data from source;", ""},
		{"T2", t2, "	console.log(test);\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeImport(tt.args)
			if got != tt.want {
				t.Errorf("removeImport() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
