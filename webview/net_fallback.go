//go:build !linux && !windows

package webview

import (
	"net"
)

func HasNetStats() error {
	return ErrNotImplemented
}

func GetNetStats(interfaces []net.Interface, dir string) []float64 {
	return make([]float64, len(interfaces))
}
