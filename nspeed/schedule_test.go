package nspeed_test

import (
	"nspeed/nspeed"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseSchedules(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		s       string
		want    []*nspeed.Schedule
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"zero", "0", []*nspeed.Schedule{{0, 0}}, false},
		{"zerozero", "0,0", []*nspeed.Schedule{{0, 0}}, false},
		{"zeroonesec", "0,1s", []*nspeed.Schedule{{0, time.Second}}, false},
		{"missinglast", "0,1s,2s", []*nspeed.Schedule{{0, time.Second}, {2 * time.Second, 0}}, false},
		{"full", "0,1s,2s,3s,4h,5m", []*nspeed.Schedule{
			{0, time.Second},
			{2 * time.Second, 3 * time.Second},
			{4 * time.Hour, 5 * time.Minute},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := nspeed.ParseSchedules(tt.s)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("ParseSchedules() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("ParseSchedules() succeeded unexpectedly")
			}
			// TODO: update the condition below to compare got with tt.want.
			require.Equal(t, tt.want, got)
		})
	}
}
