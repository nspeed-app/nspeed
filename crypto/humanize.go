// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package crypto

import "fmt"

// TLS stuff
// adapted from/inspired by https://github.com/signalsciences/tlstext

// tls "version to name" is not in std lib ?!
var tlsversionMap = map[uint16]struct {
	shortName    string
	friendlyName string
}{
	0:      {"", ""},
	0x0300: {"SSL30", "SSL 3.0"},
	0x0301: {"TLS10", "TLS 1.0"},
	0x0302: {"TLS11", "TLS 1.1"},
	0x0303: {"TLS12", "TLS 1.2"},
	0x0304: {"TLS13", "TLS 1.3"},
}

// TLSVersionName return TLS version name
func TLSVersionName(x uint16) string {
	s, ok := tlsversionMap[x]
	if !ok {
		return fmt.Sprintf("%04x", x)
	}
	return s.shortName
}

// TLSVersionName return TLS version user friendly name
func TLSVersionFriendlyName(x uint16) string {
	s, ok := tlsversionMap[x]
	if !ok {
		return fmt.Sprintf("%04x", x)
	}
	return s.friendlyName
}
