//go:build !linux && !windows && !darwin

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

func GetSockOpt(fd uintptr, value int) (int, error) {
	return 0, ErrNotImplemented
}
