// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// parts are from https://go.dev/
// SPDX-License-Identifier: BSD-3-Clause

package anonymize

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
)

const hexDigit = "0123456789abcdef"

func hexString(b []byte) string {
	s := make([]byte, len(b)*2)
	for i, tn := range b {
		s[i*2], s[i*2+1] = hexDigit[tn>>4], hexDigit[tn&0xf]
	}
	return string(s)
}

// Convert i to a hexadecimal string. Leading zeros are not printed.
func appendHex(dst []byte, i uint32) []byte {
	if i == 0 {
		return append(dst, '0')
	}
	for j := 7; j >= 0; j-- {
		v := i >> uint(j*4)
		if v > 0 {
			dst = append(dst, hexDigit[v&0xf])
		}
	}
	return dst
}

const (
	FormatV4full      = "1234"
	FormatV4First     = "1"
	FormatV4FirstLast = "14"

	FormatV6Full      = "12345678"
	FormatV6First     = "1"
	FormatV6First4    = "1234"
	FormatV6FirstLast = "18"
)

// AnonymizeIP converts an IP address to a string representation but with the option to obscure parts of the address.
//
// The function supports IPv4 and IPv6 addresses and returns one of the following:
//   - "<nil>" if the input IP has zero length.
//   - A dotted-decimal IPv4 string (e.g., "192.0.2.1") for IPv4 or IPv4-mapped IPv6 addresses.
//   - An RFC 5952-compliant IPv6 string (e.g., "2001:db8::1") for valid IPv6 addresses.
//   - A hexadecimal string (without punctuation) if the IP address doesn't fit the above formats.
//
// The `formatV4` and `formatV6` parameters control which parts of the IP are visible.
// IPv4 addresses are divided into four segments (1.2.3.4), and IPv6 into eight (1:2:3:4:5:6:7:8).
// Segments not included in `formatV4` or `formatV6` are replaced with "xxx" (for IPv4) or "xxxx" (for IPv6).
//
// Examples:
//   - With `formatV4="12"`, "192.168.12.34" becomes "192.168.xxx.xxx".
//   - With `formatV6="18"`, "fe80::abcd:efab:1234:5678" becomes "fe80::xxxx:xxxx:xxxx:5678".
//
// This function and its helper functions are adapted from the net.IP.
//
// () implementation in the Go standard library.
func AnonymizeIP(ip net.IP, formatV4 string, formatV6 string) string {
	p := ip

	if len(ip) == 0 {
		return "<nil>"
	}

	// If IPv4, use dotted notation.
	if p4 := p.To4(); len(p4) == net.IPv4len {
		s := ""
		for n := 1; n <= 4; n++ {
			if strings.Contains(formatV4, strconv.Itoa(n)) {
				s += strconv.Itoa(int(p4[n-1]))
			} else {
				s += "xxx"
			}
			if n < 4 {
				s += "."
			}
		}
		return s
	}
	if len(p) != net.IPv6len {
		return "?" + hexString(ip)
	}

	// Find longest run of zeros.
	e0 := -1
	e1 := -1
	for i := 0; i < net.IPv6len; i += 2 {
		j := i
		for j < net.IPv6len && p[j] == 0 && p[j+1] == 0 {
			j += 2
		}
		if j > i && j-i > e1-e0 {
			e0 = i
			e1 = j
			i = j
		}
	}
	// The symbol "::" MUST NOT be used to shorten just one 16 bit 0 field.
	if e1-e0 <= 2 {
		e0 = -1
		e1 = -1
	}

	const maxLen = len("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
	b := make([]byte, 0, maxLen)

	// Print with possible :: in place of run of zeros
	for i := 0; i < net.IPv6len; i += 2 {
		if i == e0 {
			b = append(b, ':', ':')
			i = e1
			if i >= net.IPv6len {
				break
			}
		} else if i > 0 {
			b = append(b, ':')
		}
		if strings.Contains(formatV6, strconv.Itoa(i/2+1)) {
			b = appendHex(b, (uint32(p[i])<<8)|uint32(p[i+1]))
		} else {
			b = append(b, []byte("xxxx")...)
		}
	}
	return string(b)
}

func AnonymizeNetIP(ip netip.Addr, formatV4 string, formatV6 string) string {
	s := AnonymizeIP(ip.AsSlice(), formatV4, formatV6)
	if ip.Zone() != "" {
		s += "%" + ip.Zone()
	}
	return s
}

func AnonymizeIPNet(n *net.IPNet, formatV4 string, formatV6 string) string {
	if n == nil {
		return "<nil>"
	}
	nn, m := networkNumberAndMask(n)
	if nn == nil || m == nil {
		return "<nil>"
	}
	l := simpleMaskLength(m)
	s := AnonymizeIP(nn, formatV4, formatV6)
	if l == -1 {
		return s + "/" + m.String()
	}
	return s + "/" + Uitoa(uint(l))
}

func networkNumberAndMask(n *net.IPNet) (ip net.IP, m net.IPMask) {
	if ip = n.IP.To4(); ip == nil {
		ip = n.IP
		if len(ip) != net.IPv6len {
			return nil, nil
		}
	}
	m = n.Mask
	switch len(m) {
	case net.IPv4len:
		if len(ip) != net.IPv4len {
			return nil, nil
		}
	case net.IPv6len:
		if len(ip) == net.IPv4len {
			m = m[12:]
		}
	default:
		return nil, nil
	}
	return
}

// If mask is a sequence of 1 bits followed by 0 bits,
// return the number of 1 bits.
func simpleMaskLength(mask net.IPMask) int {
	var n int
	for i, v := range mask {
		if v == 0xff {
			n += 8
			continue
		}
		// found non-ff byte
		// count 1 bits
		for v&0x80 != 0 {
			n++
			v <<= 1
		}
		// rest must be 0 bits
		if v != 0 {
			return -1
		}
		for i++; i < len(mask); i++ {
			if mask[i] != 0 {
				return -1
			}
		}
		break
	}
	return n
}

// Itoa converts val to a decimal string.
func Itoa(val int) string {
	if val < 0 {
		return "-" + Uitoa(uint(-val))
	}
	return Uitoa(uint(val))
}

// Uitoa converts val to a decimal string.
func Uitoa(val uint) string {
	if val == 0 { // avoid string allocation
		return "0"
	}
	var buf [20]byte // big enough for 64bit value base 10
	i := len(buf) - 1
	for val >= 10 {
		q := val / 10
		buf[i] = byte('0' + val - q*10)
		i--
		val = q
	}
	// val < 10
	buf[i] = byte('0' + val)
	return string(buf[i:])
}
