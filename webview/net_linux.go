package webview

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

func HasNetStats() error {
	return nil
}

//todo: use https://github.com/prometheus/procfs ?
// see also https://github.com/prometheus/node_exporter

// return rx/tx counters for interfaces - Linux version
func GetNetStats(interfaces []net.Interface, dir string) []float64 {
	var values []float64
	for _, itf := range interfaces {
		var value int
		file, err := os.Open(fmt.Sprintf("/sys/class/net/%s/statistics/%s_bytes", itf.Name, dir))
		if err != nil {
			value = -1
		} else {
			_, _ = fmt.Fscanf(file, "%d", &value)
		}
		values = append(values, float64(value))
	}
	return values
}

func GetSockOpt(fd uintptr, value int) (int, error) {
	val, err := syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, value)
	if err != nil {
		return 0, err
	}
	return val, nil
}
