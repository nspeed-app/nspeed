package metrics // should rename to avoid confusion with runtime/metrics and otel later

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/shirou/gopsutil/v4/cpu"
)

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

	var cpuload strings.Builder
	cpuload.WriteString("|")
	var headers strings.Builder
	headers.WriteString("|")
	for i, pc := range pcts {
		v := math.Round(pc)
		color := green
		if v > 50 {
			color = yellow
		}
		if v > 80 {
			color = red
		}
		headers.WriteString(fmt.Sprintf("%3s|", "c"+strconv.Itoa(i)))
		cpuload.WriteString(fmt.Sprintf("%s%3v%s|", color, v, reset))

	}
	return cpuload.String(), headers.String()
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

// CPUMetric - nspeed internal cpu metric
type CPUMetric struct {
	Pcts       []float64
	ActiveJobs int
}

var printHeaders sync.Once

var MetricNames = []string{
	"/cpu/classes/gc/pause:cpu-seconds",
	"/cpu/classes/gc/total:cpu-seconds",
}

var mtrSample []metrics.Sample

func PrintCPU(ctx context.Context, m *CPUMetric, log *zerolog.Logger, color bool) {
	cpuload, headers, _ := PrintCPULoad(m.Pcts, color)
	printHeaders.Do(func() {
		log.Log().Msgf("%s jobs | threads| gcpause | gctotal", headers)
		mtrSample = make([]metrics.Sample, len(MetricNames))
		for i, name := range MetricNames {
			mtrSample[i].Name = name
		}
	})

	// Sample the metric.
	metrics.Read(mtrSample)
	for i, sample := range mtrSample {
		if sample.Value.Kind() == metrics.KindBad {
			log.Warn().Msgf("metric %q no longer supported", MetricNames[i])
		}
	}
	log.Log().Msgf("%s %-4d | %-4d   | %-.4f | %-.4f", cpuload, m.ActiveJobs,
		runtime.NumGoroutine(),
		mtrSample[0].Value.Float64(),
		mtrSample[1].Value.Float64())
}
