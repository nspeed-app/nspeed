package webview

import (
	"fmt"
	"net"
	"os"
)

// return rx/tx counters for interfaces - Linux version
func GetNetStats(interfaces []net.Interface, dir string) []float64 {
	var values []float64
	for _, itf := range interfaces {
		var value int
		file, err := os.Open(fmt.Sprintf("/sys/class/net/%s/statistics/%s_bytes", itf.Name, dir))
		if err != nil {
			value = -1
		} else {
			fmt.Fscanf(file, "%d", &value)
		}
		values = append(values, float64(value))
	}
	return values
}
