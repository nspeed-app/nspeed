package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

var mp *metric.MeterProvider

func OTELInit(ctx context.Context, name, version string, w io.Writer, interval time.Duration) {

	// Print with a JSON encoder that indents with two spaces.
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	exp, err := stdoutmetric.New(
		stdoutmetric.WithEncoder(enc),
		stdoutmetric.WithoutTimestamps(),
	)
	if err != nil {
		panic(err)
	}

	//
	res := resource.NewSchemaless(
		semconv.ServiceName(name),
		semconv.ServiceVersion(version),
	)

	// Register the exporter with an SDK via a periodic reader.
	mp = metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(exp, metric.WithInterval(interval))),
	)
	otel.SetMeterProvider(mp)
}

func OTELShutdown(ctx context.Context) error {
	if mp != nil {
		return mp.Shutdown(ctx)
	}
	return fmt.Errorf("no MeterProvider")
}
