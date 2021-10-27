package main

import (
	"fmt"
	"math"
	"runtime"
	"time"

	cpu1 "github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/v3/cpu"
)

// global const
const colorReset = "\033[0m"
const colorRed = "\033[31m"
const colorGreen = "\033[32m"
const colorYellow = "\033[33m"

func GetCPULoad(interval time.Duration, colorize bool) (string, error) {

	r := ""

	pcts, err := cpu1.Percent(interval, true)
	if err != nil {
		r = "P1:" + err.Error()
	} else {
		r = Colorize(pcts, colorize)
	}
	p3, err := cpu.Percent(interval, true)
	if err != nil {
		r = r + "P3:" + err.Error()
	} else {
		r = r + Colorize(p3, colorize)
	}
	return r, nil
}

func Colorize(pcts []float64, colorize bool) string {
	red, green, yellow, reset := colorRed, colorGreen, colorYellow, colorReset
	if !colorize {
		red, green, yellow, reset = "", "", "", ""
	}

	cpuload := "|"
	for _, pc := range pcts {
		v := math.Round(pc)
		color := green
		if v > 50 {
			color = yellow
		}
		if v > 80 {
			color = red
		}
		cpuload += fmt.Sprintf("%s%3v%s|", color, v, reset)
	}
	return cpuload
}
func main() {
	for {
		load, err := GetCPULoad(0, false)
		if err != nil {
			fmt.Printf("%v\n", err)
		}
		fmt.Printf("%s, active goroutines: %d\n", load, runtime.NumGoroutine())
		time.Sleep(time.Second * 1)
	}

}
