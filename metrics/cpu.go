package metrics

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/shirou/gopsutil/v4/cpu"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
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

// CPUMetric - nspeed internal cpu metric
type CPUMetric struct {
	Pcts       []float64
	ActiveJobs int
}

// CPUOtel - otel cpu metric
type CPUOtel struct {
	meter   metric.Meter
	cpu     metric.Float64Gauge
	gcCount metric.Int64Gauge
}

func NewMetricsCPU() *CPUOtel {
	meter := otel.Meter("nspeed/metric")
	cpu, _ := meter.Float64Gauge("metric.cpu.avg", metric.WithDescription("cpu average"))
	gc, _ := meter.Int64Gauge("metric.go.gc", metric.WithDescription("Go GC calls"))
	om := CPUOtel{
		meter:   meter,
		cpu:     cpu,
		gcCount: gc,
	}
	return &om
}

var gcStats debug.GCStats

func (c *CPUOtel) RecordCPU(ctx context.Context, m *CPUMetric, log *zerolog.Logger, color bool) {
	if m == nil {
		return
	}
	mean := 0.0
	for _, cpu := range m.Pcts {
		mean += cpu
	}

	mean /= float64(len(m.Pcts))
	c.cpu.Record(ctx, mean)
	debug.ReadGCStats(&gcStats)
	c.gcCount.Record(ctx, gcStats.NumGC)

	if log == nil {
		return
	}

	cpuload, headers, _ := PrintCPULoad(m.Pcts, color)
	var printHeaders sync.Once
	printHeaders.Do(func() {
		log.Log().Msgf("%s jobs | threads|", headers)
	})
	log.Log().Msgf("%s %-4d | %-4d   |", cpuload, m.ActiveJobs, runtime.NumGoroutine())
}
