package nspeed

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
	} // todo : add more tests

	for _, table := range tables {
		y, err := ParseByteUnits(table.x)
		if y != table.y || (table.hasError != (err != nil)) {
			t.Errorf("ParseByteUnits of (%s) was incorrect, got: %d,%t want: %d,%t", table.x, y, err != nil, table.y, table.hasError)
		}
	}
}
