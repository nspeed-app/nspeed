package utils

import (
	"net"
	"net/netip"
	"testing"
)

func TestAnonymizeIP(t *testing.T) {
	type args struct {
		ip       net.IP
		formatV4 string
		formatV6 string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// ipv4
		{"IPv4allrouter full", args{ip: net.IPv4allrouter, formatV4: FormatV4full, formatV6: ""}, "224.0.0.2"},
		{"IPv4allrouter first", args{ip: net.IPv4allrouter, formatV4: FormatV4First, formatV6: ""}, "224.xxx.xxx.xxx"},
		{"IPv4allrouter firs last", args{ip: net.IPv4allrouter, formatV4: FormatV4FirstLast, formatV6: ""}, "224.xxx.xxx.2"},
		// ipv6
		{"AllDHCPRelayAgentsAndServers full", args{ip: net.ParseIP("ff02::1:2"), formatV4: "", formatV6: FormatV6Full}, "ff02::1:2"},
		{"AllDHCPRelayAgentsAndServers first", args{ip: net.ParseIP("ff02::1:2"), formatV4: "", formatV6: FormatV6First}, "ff02::xxxx:xxxx"},
		{"AllDHCPRelayAgentsAndServers first 4", args{ip: net.ParseIP("ff02::1:2"), formatV4: "", formatV6: FormatV6First4}, "ff02::xxxx:xxxx"},
		{"AllDHCPRelayAgentsAndServers first last", args{ip: net.ParseIP("ff02::1:2"), formatV4: "", formatV6: FormatV6FirstLast}, "ff02::xxxx:2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnonymizeIP(tt.args.ip, tt.args.formatV4, tt.args.formatV6); got != tt.want {
				t.Errorf("AnonymizeIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnonymizeNetIP(t *testing.T) {
	s := "fe80::abcd:efab:1234:5678%eth0"
	ipl6, _ := netip.ParseAddr(s)
	type args struct {
		ip       netip.Addr
		formatV4 string
		formatV6 string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"link-local full", args{ip: ipl6, formatV4: "", formatV6: FormatV6Full}, s},
		{"link-local first", args{ip: ipl6, formatV4: "", formatV6: FormatV6First}, "fe80::xxxx:xxxx:xxxx:xxxx%eth0"},
		{"link-local first 4", args{ip: ipl6, formatV4: "", formatV6: FormatV6First4}, "fe80::xxxx:xxxx:xxxx:xxxx%eth0"},
		{"link-local first last", args{ip: ipl6, formatV4: "", formatV6: FormatV6FirstLast}, "fe80::xxxx:xxxx:xxxx:5678%eth0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnonymizeNetIP(tt.args.ip, tt.args.formatV4, tt.args.formatV6); got != tt.want {
				t.Errorf("AnonymizeNetIP() = %v, want %v", got, tt.want)
			}
		})
	}
}
