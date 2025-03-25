package crypto

import (
	"testing"
)

func TestTLSVersionName(t *testing.T) {
	tests := []struct {
		name     string
		version  uint16
		expected string
	}{
		{"Unknown", 0x9999, "9999"},
		{"SSL30", 0x0300, "SSL30"},
		{"TLS10", 0x0301, "TLS10"},
		{"TLS11", 0x0302, "TLS11"},
		{"TLS12", 0x0303, "TLS12"},
		{"TLS13", 0x0304, "TLS13"},
		{"Zero", 0, ""},
		{"Max", 65535, "ffff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := TLSVersionName(tt.version)
			if actual != tt.expected {
				t.Errorf("TLSVersionName(%04x) = %v, expected %v", tt.version, actual, tt.expected)
			}
		})
	}
}

func TestTLSVersionFriendlyName(t *testing.T) {
	tests := []struct {
		name     string
		version  uint16
		expected string
	}{
		{"Unknown", 0x9999, "9999"},
		{"SSL30", 0x0300, "SSL 3.0"},
		{"TLS10", 0x0301, "TLS 1.0"},
		{"TLS11", 0x0302, "TLS 1.1"},
		{"TLS12", 0x0303, "TLS 1.2"},
		{"TLS13", 0x0304, "TLS 1.3"},
		{"Zero", 0, ""},
		{"Max", 65535, "ffff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := TLSVersionFriendlyName(tt.version)
			if actual != tt.expected {
				t.Errorf("TLSVersionFriendlyName(%04x) = %v, expected %v", tt.version, actual, tt.expected)
			}
		})
	}
}
