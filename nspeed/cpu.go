package nspeed

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
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

func FormatCPULoad(pcts []float64, colorize bool) (string, string) {
	red, green, yellow, reset := colorRed, colorGreen, colorYellow, colorReset
	if !colorize {
		red, green, yellow, reset = "", "", "", ""
	}

	cpuload := "|"
	headers := "|"
	for i, pc := range pcts {
		v := math.Round(pc)
		color := green
		if v > 50 {
			color = yellow
		}
		if v > 80 {
			color = red
		}
		headers += fmt.Sprintf("%3s|", "c"+strconv.Itoa(i))
		cpuload += fmt.Sprintf("%s%3v%s|", color, v, reset)

	}
	return cpuload, headers
}

func PrintCPULoad(pcts []float64, colorize bool) (string, string, int) {
	s, h := FormatCPULoad(pcts, colorize)
	return s, h, len(pcts)
}

// todo: get cpu values from a unique goroutine/Once
func GetCPULoad(interval time.Duration, percpu bool) ([]float64, error) {
	pcts, err := cpu.Percent(interval, percpu)
	if err != nil {
		return nil, err
	}
	return pcts, nil
}

type CPUMetric struct {
	Pcts       []float64
	ActiveJobs int
}
