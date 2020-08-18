package nspeed

import "fmt"

// shamelessly copied from : https://programming.guide/go/formatting-byte-size-to-human-readable-format.html

// ByteCountDecimal format byte size to human readable format (decimal units)
func ByteCountDecimal(b int64, u string) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d %s", b, u)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %c%s", float64(b)/float64(div), "kMGTPE"[exp], u)
}

// ByteCountBinary format byte size to human readable format (binary units)
func ByteCountBinary(b int64, u string) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d %s", b, u)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %c%s", float64(b)/float64(div), "KMGTPE"[exp], u)
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
		speed = ByteCountDecimal((int64)((float64)(totalBytesRead)*8.0/elapsedSeconds), "bps")
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
