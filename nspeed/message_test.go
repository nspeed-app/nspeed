package nspeed

import (
	"bytes"
	"reflect"
	"testing"
)

func TestL4Message(t *testing.T) {
	args := []struct {
		msg     L4Message
		wantErr bool
	}{
		{NewL4Message("PUT", 12), false},
		{L4Message{"bad cookie", 24, "get"}, true},
	}
	for _, arg := range args {
		t.Run("ReadL4Message", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := WriteL4Message(b, &arg.msg)
			if err != nil {
				t.Errorf("WriteL4Message() bad error = %v", err)
				return
			}
			got, err := ReadL4Message(b)
			if (err != nil) != arg.wantErr {
				t.Errorf("ReadL4Message() error = %v", err)
				return
			}
			if err != nil {
				return
			}
			if !reflect.DeepEqual(got, &arg.msg) {
				t.Errorf("ReadL4Message() = %v, want %v", got, arg)
			}
		})
	}
}
