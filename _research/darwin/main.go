package main

import (
	"fmt"
	"math"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

func GetCPULoad(interval time.Duration) (string, error) {

	pcts, err := cpu.Percent(interval, true)
	if err != nil {
		return "", err
	}

	cpuload := "|"
	for _, pc := range pcts {
		v := math.Round(pc)
		cpuload += fmt.Sprintf("%3v|", v)
	}
	return cpuload, nil
}
func main() {
	for {
		load, err := GetCPULoad(0)
		if err != nil {
			fmt.Printf("%v\n", err)
		}
		fmt.Printf("%s\n", load)
		time.Sleep(time.Second * 1)
	}
}
