package utils

import (
	"net/netip"
	"testing"
)

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

// warning: Gemini generated

func TestIPVersion_IsValid(t *testing.T) {
	tests := []struct {
		name string
		v    IPVersion
		want bool
	}{
		{"IPv4", 4, true},
		{"IPv6", 6, true},
		{"Any", 0, true},
		{"Invalid", -1, false},
		{"Other", 10, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.IsValid(); got != tt.want {
				t.Errorf("IPVersion.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIPVersion_String(t *testing.T) {
	tests := []struct {
		name string
		v    IPVersion
		want string
	}{
		{"IPv4", 4, "IPv4"},
		{"IPv6", 6, "IPv6"},
		{"Any", 0, ""},
		{"Invalid", -1, "invalid"},
		{"Other", 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.String(); got != tt.want {
				t.Errorf("IPVersion.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIPVersion_NumericString(t *testing.T) {
	tests := []struct {
		name string
		v    IPVersion
		want string
	}{
		{"IPv4", 4, "4"},
		{"IPv6", 6, "6"},
		{"Any", 0, ""},
		{"Invalid", -1, "invalid"},
		{"Other", 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.NumericString(); got != tt.want {
				t.Errorf("IPVersion.NumericString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseIPVersionFromString(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    IPVersion
	}{
		{"IPv4", "192.168.1.1:80", 4},
		{"IPv6", "[2001:db8::1]:80", 6},
		{"IPv6 no port", "2001:db8::1", 6},
		{"invalid", "not an ip", -1},
		{"empty", "", -1},
		{"IPv4 no port", "10.0.0.1", 4},
		{"invalid with port", "10.0.0.1:notport", -1},
		{"invalid port only", ":80", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseIPVersionFromString(tt.address); got != tt.want {
				t.Errorf("ParseIPVersionFromString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetIPVersion(t *testing.T) {
	tests := []struct {
		name    string
		address netip.Addr
		want    IPVersion
	}{
		{"IPv4", netip.MustParseAddr("192.168.1.1"), 4},
		{"IPv6", netip.MustParseAddr("2001:db8::1"), 6},
		{"Invalid", netip.Addr{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetIPVersion(tt.address); got != tt.want {
				t.Errorf("GetIPVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddIPVersionToNetwork(t *testing.T) {
	tests := []struct {
		name      string
		network   string
		ipVersion IPVersion
		want      string
	}{
		{"UDPv4", "udp", 4, "udp4"},
		{"UDPv6", "udp", 6, "udp6"},
		{"TCPv4", "tcp", 4, "tcp4"},
		{"TCPv6", "tcp", 6, "tcp6"},
		{"NoVersion", "udp", 0, "udp"},
		{"NoVersionTCP", "tcp", 0, "tcp"},
		{"other protocol", "icmp", 4, "icmp4"},
		{"other protocol", "icmp", 6, "icmp6"},
		{"other protocol", "icmp", 0, "icmp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AddIPVersionToNetwork(tt.network, tt.ipVersion); got != tt.want {
				t.Errorf("AddIPVersionToNetwork() = %v, want %v", got, tt.want)
			}
		})
	}
}
