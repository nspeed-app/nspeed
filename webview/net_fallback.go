//go:build !linux && !windows
// +build !linux,!windows

package webview

import "net"

func HasNetStats() {
	return common.ErrNotImplementedError
}

func GetNetStats(interfaces []net.Interface, dir string) []float64 {
	return []
}
