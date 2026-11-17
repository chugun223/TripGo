package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type PositionMetrics struct {
	Saved           metric.Int64Counter
	StaleChecks     metric.Int64Counter
	Requests        metric.Int64Counter
	RequestDuration metric.Float64Histogram
	ScanRows        metric.Int64Histogram
	registration    metric.Registration
}

type positionPoolStats interface {
	QueueSize() int64
	ActiveWorkers() int64
}

func NewPositionMetrics() (*PositionMetrics, error) {
	meter := otel.Meter("tripgo/positions")
	saved, err := meter.Int64Counter("tripgo.positions.saved", metric.WithUnit("{position}"))
	if err != nil {
		return nil, fmt.Errorf("create saved positions counter: %w", err)
	}
	checks, err := meter.Int64Counter("tripgo.stale_checks", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create stale checks counter: %w", err)
	}
	requests, err := meter.Int64Counter("tripgo.position_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, fmt.Errorf("create position requests counter: %w", err)
	}
	duration, err := meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5))
	if err != nil {
		return nil, fmt.Errorf("create position requests histogram: %w", err)
	}
	rows, err := meter.Int64Histogram("tripgo.stale_checks.rows", metric.WithUnit("{trip}"),
		metric.WithDescription("Rows read during one scanner tick"),
		metric.WithExplicitBucketBoundaries(0, 10, 100, 250, 500, 1000))
	if err != nil {
		return nil, fmt.Errorf("create scanner rows histogram: %w", err)
	}
	return &PositionMetrics{Saved: saved, StaleChecks: checks, Requests: requests, RequestDuration: duration, ScanRows: rows}, nil
}

func (m *PositionMetrics) RegisterPool(pool positionPoolStats) error {
	meter := otel.Meter("tripgo/positions")
	queue, err := meter.Int64ObservableGauge("tripgo.position_requests.queue.size", metric.WithUnit("{task}"))
	if err != nil {
		return fmt.Errorf("create position queue gauge: %w", err)
	}
	workers, err := meter.Int64ObservableGauge("tripgo.position_requests.workers.active", metric.WithUnit("{worker}"))
	if err != nil {
		return fmt.Errorf("create active workers gauge: %w", err)
	}
	registration, err := meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		observer.ObserveInt64(queue, pool.QueueSize())
		observer.ObserveInt64(workers, pool.ActiveWorkers())
		return nil
	}, queue, workers)
	if err != nil {
		return fmt.Errorf("register position pool gauges: %w", err)
	}
	m.registration = registration
	return nil
}

func (m *PositionMetrics) Close() error {
	if m.registration == nil {
		return nil
	}
	return m.registration.Unregister()
}
