// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// parts are from https://go.dev/
// SPDX-License-Identifier: BSD-3-Clause

package utils // copied from "golang.org/x/net/internal/iana"

const (
	ProtocolIP       = 0  // IPv4 encapsulation, pseudo protocol number
	ProtocolICMP     = 1  // Internet Control Message
	ProtocolIPv4     = 4  // IPv4 encapsulation
	ProtocolTCP      = 6  // Transmission Control
	ProtocolUDP      = 17 // User Datagram
	ProtocolIPv6     = 41 // IPv6 encapsulation
	ProtocolIPv6ICMP = 58 // ICMP for IPv6
)

const (
	AddrFamilyIPv4 = 1 // IP (IP version 4)
	AddrFamilyIPv6 = 2 // IP6 (IP version 6)
)
