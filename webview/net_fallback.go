//go:build !linux && !windows
// +build !linux,!windows

package webview

import "net"

func HasNetStats() error {
	return common.ErrNotImplementedError
}

func GetNetStats(interfaces []net.Interface, dir string) []float64 {
	return make([]float64, len(interfaces))
}
