// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause
package humanize

import (
	"testing"
)

func TestParseByteUnits(t *testing.T) {
	tables := []struct {
		x        string
		y        uint64
		hasError bool
	}{
		{"1", 1, false},
		{"-1", 0, true},
		{"1k", 1000, false},
		{"1K", 1024, false},
		{"1m", 1000 * 1000, false},
		{"1M", 1024 * 1024, false},
		{"1g", 1000 * 1000 * 1000, false},
		{"1G", 1024 * 1024 * 1024, false},
		{"1.2k", 1200, false},
		{"1.2K", 1228, false},
	} // todo : add more tests

	for _, table := range tables {
		y, err := ParseByteUnits(table.x)
		if y != table.y || (table.hasError != (err != nil)) {
			t.Errorf("ParseByteUnits of (%s) was incorrect, got: %d,%t want: %d,%t", table.x, y, err != nil, table.y, table.hasError)
		}
	}
}

func TestByteCountBinary(t *testing.T) {
	tables := []struct {
		x int64
		y string
	}{
		{1, "1 "},
		{1000, "1000 "},
		{1024, "1.0 Ki"},
		{1024 * 1024, "1.0 Mi"},
		{1024*1024 + 100*1024, "1.1 Mi"},
		{0, "0 "},
	}
	for _, table := range tables {
		s := ByteCountBinary(table.x)
		if s != table.y {
			t.Errorf("ByteCountBinary of (%d) was incorrect, got: |%s| want |%s|", table.x, s, table.y)
		}
	}
}

func TestByteCountDecimal(t *testing.T) {
	tables := []struct {
		x int64
		y string
	}{
		{1, "1 "},
		{999, "999 "},
		{1000, "1.0 k"},
		{1024, "1.0 k"},
		{1024 * 1024, "1.0 M"},
		{1024*1024 + 100*1024, "1.2 M"},
		{0, "0 "},
	}
	for _, table := range tables {
		s := ByteCountDecimal(table.x)
		if s != table.y {
			t.Errorf("ByteCountBinary of (%d) was incorrect, got: |%s| want |%s|", table.x, s, table.y)
		}
	}
}
