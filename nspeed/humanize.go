package nspeed

import (
	"fmt"
	"strconv"
	"strings"
)

// shamelessly copied from : https://programming.guide/go/formatting-byte-size-to-human-readable-format.html
func byteCount(b int64, u string, unit int64, units string) string {
	if b < unit {
		return fmt.Sprintf("%d ", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	if exp >= len(units) {
		return fmt.Sprintf("%d", b)
	}
	return fmt.Sprintf("%.1f %c%s", float64(b)/float64(div), units[exp], u)
}

// ByteCountDecimal format byte size to human readable format (decimal units)
func ByteCountDecimal(b int64) string {
	return byteCount(b, "", 1000, "kMGTPE")
}

// ByteCountBinary format byte size to human readable format (binary units)
func ByteCountBinary(b int64) string {
	return byteCount(b, "i", 1024, "KMGTPE")
}

// parse bytes units - "<value>kmgtpe" powers of 2 (B), "<value>KMGTPE" powers of 10 (iB)
const units = "kmgtpe"

func ParseByteUnits(s string) (uint64, error) {
	m := uint64(1)
	u := s[len(s)-1:]
	i := strings.Index(units, strings.ToLower(u))
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
	val, err := strconv.ParseUint(s, 10, 64)
	return m * val, err
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
