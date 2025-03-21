package metrics

import (
	"context"
	"runtime/debug"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

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

func (c *CPUOtel) RecordCPU(ctx context.Context, m *CPUMetric) {
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
}
