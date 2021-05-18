package nspeed

import (
	"fmt"
	"strconv"
	"strings"
)

const binaryUnits = "kmgtpe"
const decimalUnits = "KMGTPE"

// regexp
// use https://regoio.herokuapp.com/ to test
const ByteUnitsRegexp = "[0-9]+[kmgtpeKMGTPE]?"

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

// ByteCountDecimal format byte size to human readable format (decimal units)
// suitable to append the unit name after (B, bps, etc)
func ByteCountDecimal(b int64) string {
	s, u := byteCount(b, 1000, "kMGTPE")
	return s + " " + u
}

// ByteCountBinary format byte size to human readable format (binary units)
// suitable to append the unit name after (B, bps, etc)
func ByteCountBinary(b int64) string {
	s, u := byteCount(b, 1024, decimalUnits)
	if len(u) > 0 {
		return s + " " + u + "i"
	} else {
		return s + " "
	}
}

// FormatByteDecimalUnits format byte size to human readable format (decimal units)
// suitable to be parsed by ParseByteUnits
func FormatByteDecimalUnits(b int64) string {
	s, u := byteCount(b, 1000, binaryUnits)
	return s + u
}

// FormatByteDecimalUnits format byte size to human readable format (decimal units)
// suitable to be parsed by ParseByteUnits
func FormatByteBinaryUnits(b int64) string {
	s, u := byteCount(b, 1024, decimalUnits)
	return s + u
}

// parse bytes units - "<value>kmgtpe" powers of 2 (B), "<value>KMGTPE" powers of 10 (iB)
func ParseByteUnits(s string) (uint64, error) {
	m := uint64(1)
	u := s[len(s)-1:]
	i := strings.Index(binaryUnits, strings.ToLower(u))
	if i > -1 {
		s = s[:len(s)-1]
		unit := uint64(1000)
		if u == strings.ToUpper(u) {
			unit = uint64(1024)
		}
		for x := 0; x <= i; x++ {
			m = m * unit
		}
	}
	val, err := strconv.ParseFloat(s, 64)
	if err == nil {
		if val < 0.0 {
			fmt.Println("NEG")
			return 0, strconv.ErrSyntax
		}
		return uint64(float64(m) * val), err
	}
	return 0, err
}

// FormatBitperSecond format bit per seconds in human readable format
func FormatBitperSecond(elapsedSeconds float64, totalBytesRead int64) string {
	// nyi - fix me
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered from divide by zero")
		}
	}()
	speed := "(too fast)"
	if elapsedSeconds > 0 {
		speed = ByteCountDecimal((int64)((float64)(totalBytesRead)*8.0/elapsedSeconds)) + "bps"
	}
	return speed
}

// TLS stuff
// copied from/inspired by https://github.com/signalsciences/tlstext

// tls version to name is not in std lib ?!
var tlsversionMap = map[uint16]string{
	0x0300: "SSL30",
	0x0301: "TLS10",
	0x0302: "TLS11",
	0x0303: "TLS12",
	0x0304: "TLS13",
}

// TLSVersionName return TLS version name
func TLSVersionName(x uint16) string {
	s, ok := tlsversionMap[x]
	if !ok {
		return fmt.Sprintf("%04x", x)
	}
	return s
}
