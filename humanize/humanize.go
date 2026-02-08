// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

// Pacakge humanize provides various functions for human readable output/input
package humanize

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// prefixes
const lcPrefixes = "kmgtpe"
const ucPrefixes = "KMGTPE"
const decPrefixes = "kMGTPE"

// regexp to parse prefixes (kilo,mega,giga,etc)
const ByteUnitsRegexp = "[0-9]+[kmgtpeKMGTPE]?"

// ByteCountDecimal formats a number b of bytes to human readable format (decimal units, powers of 10)
// suitable to append a unit name after (B, bps, etc).
func ByteCountDecimal(b int64) string {
	s, u := byteCount(b, 1000, decPrefixes)
	return s + " " + u
}

// ByteCountBinary formats a number b of bytes to human readable format (binary units, powers of 2)
// suitable to append the unit name after (B, bps, etc)
func ByteCountBinary(b int64) string {
	s, u := byteCount(b, 1024, ucPrefixes)
	if len(u) > 0 {
		return s + " " + u + "i"
	}
	return s + " "

}

// FormatByteDecimalUnits formats a number b of bytes to human readable format (decimal units)
// suitable to be parsed by ParseByteUnits
func FormatByteDecimalUnits(b int64) string {
	s, u := byteCount(b, 1000, lcPrefixes)
	return s + u
}

// FormatByteDecimalUnits formats a number b of bytes to human readable format (binary units)
// suitable to be parsed by ParseByteUnits
func FormatByteBinaryUnits(b int64) string {
	s, u := byteCount(b, 1024, ucPrefixes)
	return s + u
}

// shamelessly copied from : https://programming.guide/go/formatting-byte-size-to-human-readable-format.html
func byteCount(b int64, unit int64, units string) (string, string) {
	if b < unit {
		return fmt.Sprintf("%d", b), ""
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	if exp >= len(units) {
		return fmt.Sprintf("%d", b), ""
	}
	return fmt.Sprintf("%.1f", float64(b)/float64(div)), units[exp : exp+1]
}

// ParseByteUnits parses the string s for human friendly units of 2 or 10:
//   - "<value>kmgtpe" powers of 2 (B) or
//   - "<value>KMGTPE" powers of 10 (iB)
//
// where value must be a valid number
// if s is empty zero is returned.
// example:
//
//	ParseByteUnits("10k") will return 10x1000 = 10000 (for instance 10 KB)
//	ParseByteUnits("10K") will return 10x1024 = 10240 (for instance 10 KiB)
func ParseByteUnits(s string) (uint64, error) {
	if s == "" || s == "0" {
		return 0, nil
	}
	m := uint64(1)
	u := s[len(s)-1:]
	i := strings.Index(lcPrefixes, strings.ToLower(u))
	if i > -1 {
		s = s[:len(s)-1]
		unit := uint64(1000)
		if u == strings.ToUpper(u) {
			unit = uint64(1024)
		}
		for x := 0; x <= i; x++ {
			m *= unit
		}
	}
	val, err := strconv.ParseFloat(s, 64)
	if err == nil {
		if val < 0.0 {
			return 0, strconv.ErrSyntax
		}
		return uint64(float64(m) * val), err
	}
	return 0, err
}

// FormatBitperSecond formats a number of bytes and a duration into "bit per second" in human readable format.
// The result uses the standard decimal units of 1000 (k,M,G,etc).
func FormatBitperSecond(bytes int64, duration time.Duration) string {
	bps := BitPerSecondFromInt64(bytes, duration)
	if bps != -1 {
		return ByteCountDecimal(bps) + "bps"
	}
	return "(too fast)"
}

// BitPerSecondFromUInt64 converts a number of int64 bytes and a duration to "bit per second".
// Return -1 if duration is below 1ms or negative number of bytes
func BitPerSecondFromInt64(bytes int64, duration time.Duration) int64 {
	if bytes < 0 {
		return -1
	}
	return BitPerSecondFromUInt64(uint64(bytes), duration)
}

// BitPerSecondFromUInt64 converts a number of uint64 bytes and a duration to "bit per second".
// Return -1 if duration is below 1ms.
func BitPerSecondFromUInt64(bytes uint64, duration time.Duration) int64 {
	if duration.Milliseconds() > 0 {
		return int64((float64(bytes)) * 8.0 / duration.Seconds())
	}
	return -1
}
