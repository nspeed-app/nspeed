package utils

import "testing"

func TestParseAddressWithOptionnalPort(t *testing.T) {
	type args struct {
		hostport string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		want1   int
		wantErr bool
	}{
		{"t1", args{"localhost:80"}, "localhost", 80, false},
		{"t2", args{":80"}, "", 80, false},
		{"t3", args{"localhost"}, "localhost", 0, false},
		{"t4", args{"10.0.0:23"}, "10.0.0", 23, false},
		{"t5", args{"Ethernet 1:23"}, "Ethernet 1", 23, false},
		{"t6", args{"8.8.8.8:123456"}, "8.8.8.8", 0, true},
		{"t7", args{"8.8.8.8:test"}, "8.8.8.8", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := ParseAddressWithOptionnalPort(tt.args.hostport)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAddressWithOptionnalPort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseAddressWithOptionnalPort() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("ParseAddressWithOptionnalPort() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
