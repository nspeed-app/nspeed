package metric

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type otelmetric struct {
	meter   metric.Meter
	rxspeed metric.Float64Gauge
	txspeed metric.Float64Gauge
}

func NewMetricsData() *otelmetric {
	meter := otel.Meter("nspeed/metric")
	rxs, _ := meter.Float64Gauge("metric.rx.speed", metric.WithDescription("read speed"), metric.WithUnit("bps"))
	txs, _ := meter.Float64Gauge("metric.tx.speed", metric.WithDescription("write speed"), metric.WithUnit("bps"))
	om := otelmetric{
		meter:   meter,
		rxspeed: rxs,
		txspeed: txs,
	}
	return &om
}

func (m *otelmetric) Record(ctx context.Context, rx, tx float64) {
	m.rxspeed.Record(ctx, rx)
	m.txspeed.Record(ctx, tx)
}
