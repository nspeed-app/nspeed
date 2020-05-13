package nspeed

import "fmt"

// shamelessly stolen from : https://programming.guide/go/formatting-byte-size-to-human-readable-format.html

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
