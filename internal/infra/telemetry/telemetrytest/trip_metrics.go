package telemetrytest

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

func NewTripMetrics(t *testing.T) (*telemetry.TripMetrics, *metric.ManualReader) {
	t.Helper()

	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))

	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)

	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)

		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})

	tripMetrics, err := telemetry.NewTripMetrics()
	if err != nil {
		t.Fatalf("create trip metrics: %v", err)
	}

	return tripMetrics, reader
}

func CounterValue(
	t *testing.T,
	reader *metric.ManualReader,
	name string,
) int64 {
	t.Helper()

	var data metricdata.ResourceMetrics

	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric %s is not an int64 sum", name)
			}

			var total int64

			for _, point := range sum.DataPoints {
				total += point.Value
			}

			return total
		}
	}

	return 0
}
