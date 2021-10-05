//go:build !linux && !windows
// +build !linux,!windows

package webview

import "net"

func GetNetStats(interfaces []net.Interface, dir string) []float64 {
	return nil, common.ErrNotImplementedError
}
