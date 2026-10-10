package nspeed

import "testing"

func TestIPerf3_make_cookie(t *testing.T) {
	t.Run("make cookie length", func(t *testing.T) {
		if got := IPerf3_make_cookie(); len(got) != COOKIE_SIZE {
			t.Errorf("len(IPerf3_make_cookie()) = %v, want %v", got, COOKIE_SIZE)
		}
	})
}
