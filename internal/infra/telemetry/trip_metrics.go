package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type ActiveTripCounter interface {
	CountActive(ctx context.Context) (int64, error)
}

type TripMetrics struct {
	Created   metric.Int64Counter
	Completed metric.Int64Counter

	registration metric.Registration
}

func NewTripMetrics() (*TripMetrics, error) {
	meter := otel.Meter("tripgo/trips")

	created, err := meter.Int64Counter(
		"tripgo.trips.created",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Number of successfully created trips"),
	)
	if err != nil {
		return nil, fmt.Errorf("create trips created counter: %w", err)
	}

	completed, err := meter.Int64Counter(
		"tripgo.trips.completed",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Number of successfully completed trips"),
	)
	if err != nil {
		return nil, fmt.Errorf("create trips completed counter: %w", err)
	}

	return &TripMetrics{
		Created:   created,
		Completed: completed,
	}, nil
}

func (m *TripMetrics) RegisterActive(
	repository ActiveTripCounter,
	timeout time.Duration,
) error {
	meter := otel.Meter("tripgo/trips")

	active, err := meter.Int64ObservableGauge(
		"tripgo.trips.active",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Current number of active trips"),
	)
	if err != nil {
		return fmt.Errorf("create active trips gauge: %w", err)
	}

	registration, err := meter.RegisterCallback(
		func(ctx context.Context, observer metric.Observer) error {
			queryCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			count, err := repository.CountActive(queryCtx)
			if err != nil {
				slog.ErrorContext(
					ctx,
					"failed to count active trips",
					slog.Any("error", err),
				)
				return err
			}

			observer.ObserveInt64(active, count)

			return nil
		},
		active,
	)
	if err != nil {
		return fmt.Errorf("register active trips callback: %w", err)
	}

	m.registration = registration

	return nil
}

func (m *TripMetrics) Close() error {
	if m.registration == nil {
		return nil
	}

	return m.registration.Unregister()
}
