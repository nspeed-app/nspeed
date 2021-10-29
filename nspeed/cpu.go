package nspeed

import (
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// MaxParallelism return number of cpu cores the program can actually use
// todo: probably obsolete to do like this, just use runtime.NumCPU()
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

func FormatCPULoad(pcts []float64, colorize bool) string {
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

// todo: get cpu values from a unique goroutine/Once
func GetCPULoad(interval time.Duration, colorize bool) (string, error) {
	pcts, err := cpu.Percent(interval, true)
	if err != nil {
		return "", err
	}
	return FormatCPULoad(pcts, colorize), nil
}
