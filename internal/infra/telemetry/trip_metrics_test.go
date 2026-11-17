package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	metricdata "go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestNewTripMetrics(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(
		metric.WithReader(reader),
	)

	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)

	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	metrics, err := NewTripMetrics()
	if err != nil {
		t.Fatalf("create trip metrics: %v", err)
	}

	ctx := context.Background()

	metrics.Created.Add(ctx, 1)
	metrics.Created.Add(ctx, 1)
	metrics.Completed.Add(ctx, 1)

	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	counts := make(map[string]int64)

	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}

			for _, point := range sum.DataPoints {
				counts[m.Name] += point.Value
			}
		}
	}

	if counts["tripgo.trips.created"] != 2 {
		t.Fatalf(
			"created = %d, want 2",
			counts["tripgo.trips.created"],
		)
	}

	if counts["tripgo.trips.completed"] != 1 {
		t.Fatalf(
			"completed = %d, want 1",
			counts["tripgo.trips.completed"],
		)
	}
}

type fakeActiveTripCounter struct {
	count int64
	calls int
	err   error
}

func (f *fakeActiveTripCounter) CountActive(
	_ context.Context,
) (int64, error) {
	f.calls++
	return f.count, f.err
}

func TestActiveTripsGauge(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(
		metric.WithReader(reader),
	)

	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)

	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)

		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})

	metrics, err := NewTripMetrics()
	if err != nil {
		t.Fatalf("create trip metrics: %v", err)
	}

	repository := &fakeActiveTripCounter{count: 5}

	if err := metrics.RegisterActive(repository, time.Second); err != nil {
		t.Fatalf("register active trips gauge: %v", err)
	}

	t.Cleanup(func() {
		if err := metrics.Close(); err != nil {
			t.Errorf("close trip metrics: %v", err)
		}
	})

	var data metricdata.ResourceMetrics

	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	if repository.calls != 1 {
		t.Fatalf("CountActive calls = %d, want 1", repository.calls)
	}

	found := false

	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "tripgo.trips.active" {
				continue
			}

			gauge, ok := m.Data.(metricdata.Gauge[int64])
			if !ok {
				t.Fatal("active trips metric is not an int64 gauge")
			}

			if len(gauge.DataPoints) != 1 {
				t.Fatalf(
					"active trips data points = %d, want 1",
					len(gauge.DataPoints),
				)
			}

			if gauge.DataPoints[0].Value != 5 {
				t.Fatalf(
					"active trips = %d, want 5",
					gauge.DataPoints[0].Value,
				)
			}

			found = true
		}
	}

	if !found {
		t.Fatal("active trips gauge not found")
	}
}

func TestActiveTripsGaugeRepositoryError(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(
		metric.WithReader(reader),
	)

	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)

	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)

		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})

	metrics, err := NewTripMetrics()
	if err != nil {
		t.Fatalf("create trip metrics: %v", err)
	}

	repository := &fakeActiveTripCounter{
		err: errors.New("database unavailable"),
	}

	if err := metrics.RegisterActive(repository, time.Second); err != nil {
		t.Fatalf("register active trips gauge: %v", err)
	}

	t.Cleanup(func() {
		if err := metrics.Close(); err != nil {
			t.Errorf("close trip metrics: %v", err)
		}
	})

	var data metricdata.ResourceMetrics

	_ = reader.Collect(context.Background(), &data)

	if repository.calls != 1 {
		t.Fatalf("CountActive calls = %d, want 1", repository.calls)
	}

	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "tripgo.trips.active" {
				continue
			}

			gauge, ok := m.Data.(metricdata.Gauge[int64])
			if !ok {
				t.Fatal("active trips metric is not an int64 gauge")
			}

			if len(gauge.DataPoints) != 0 {
				t.Fatal("active trips gauge must not publish a value on error")
			}
		}
	}
}

func TestActiveTripsGaugeClose(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(
		metric.WithReader(reader),
	)

	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)

	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)

		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})

	metrics, err := NewTripMetrics()
	if err != nil {
		t.Fatalf("create trip metrics: %v", err)
	}

	repository := &fakeActiveTripCounter{count: 5}

	if err := metrics.RegisterActive(repository, time.Second); err != nil {
		t.Fatalf("register active trips gauge: %v", err)
	}

	var data metricdata.ResourceMetrics

	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("first collect: %v", err)
	}

	if repository.calls != 1 {
		t.Fatalf("CountActive calls = %d, want 1", repository.calls)
	}

	if err := metrics.Close(); err != nil {
		t.Fatalf("close trip metrics: %v", err)
	}

	data = metricdata.ResourceMetrics{}

	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("second collect: %v", err)
	}

	if repository.calls != 1 {
		t.Fatalf(
			"CountActive calls after Close = %d, want 1",
			repository.calls,
		)
	}
}
