package nspeed

import (
	"fmt"
	"math"
	"runtime"

	"github.com/shirou/gopsutil/v3/cpu"
)

// MaxParallelism return number of cpu cores the program can actually use
func MaxParallelism() int {
	maxProcs := runtime.GOMAXPROCS(0)
	numCPU := runtime.NumCPU()
	if maxProcs < numCPU {
		return maxProcs
	}
	return numCPU
}

// global const
const colorReset = "\033[0m"
const colorRed = "\033[31m"
const colorGreen = "\033[32m"
const colorYellow = "\033[33m"

func GetCPULoad(colorize bool) string {
	red, green, yellow, reset := colorRed, colorGreen, colorYellow, colorReset
	if !colorize {
		red, green, yellow, reset = "", "", "", ""
	}
	pcts, _ := cpu.Percent(0, true)
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
