package observability

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/logger/loggertest"
	"github.com/chugun223/TripGo/internal/infra/telemetry/telemetrytest"
	"github.com/chugun223/TripGo/internal/model"
)

const levelError = "ERROR"

type createExecutorStub struct {
	result trip.Trip
	err    error
	calls  int
}

func (s *createExecutorStub) Execute(
	_ context.Context,
	_ model.CreateTripInput,
) (trip.Trip, error) {
	s.calls++
	return s.result, s.err
}

func TestCreateSuccess(t *testing.T) {
	logBuffer := loggertest.Capture(t)
	metrics, reader := telemetrytest.NewTripMetrics(t)

	id := uuid.New()
	next := &createExecutorStub{
		result: trip.Trip{
			ID:     id,
			Status: trip.TripStatusActive,
		},
	}

	decorator := NewCreate(next, metrics, slog.Default())

	result, err := decorator.Execute(
		context.Background(),
		model.CreateTripInput{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != id {
		t.Fatalf("unexpected trip ID: %s", result.ID)
	}

	if next.calls != 1 {
		t.Fatalf("usecase called %d times, want 1", next.calls)
	}

	count := telemetrytest.CounterValue(
		t,
		reader,
		"tripgo.trips.created",
	)
	if count != 1 {
		t.Fatalf("created trips counter = %d, want 1", count)
	}

	found := false
	for _, record := range loggertest.Records(t, logBuffer) {
		if record["msg"] != "trip created" {
			continue
		}

		found = true

		if record["level"] != "INFO" {
			t.Fatalf("unexpected log level: %v", record["level"])
		}

		if record["service"] != "trip-service" {
			t.Fatalf("unexpected service: %v", record["service"])
		}

		if record["trip_id"] != id.String() {
			t.Fatalf("unexpected trip_id: %v", record["trip_id"])
		}
	}

	if !found {
		t.Fatal("expected trip created log")
	}
}

func TestCreateErrorDoesNotIncrementMetric(t *testing.T) {
	logBuffer := loggertest.Capture(t)
	metrics, reader := telemetrytest.NewTripMetrics(t)

	expectedErr := errors.New("database unavailable")
	next := &createExecutorStub{err: expectedErr}

	decorator := NewCreate(next, metrics, slog.Default())

	_, err := decorator.Execute(
		context.Background(),
		model.CreateTripInput{},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if next.calls != 1 {
		t.Fatalf("usecase called %d times, want 1", next.calls)
	}

	count := telemetrytest.CounterValue(
		t,
		reader,
		"tripgo.trips.created",
	)
	if count != 0 {
		t.Fatalf("created trips counter = %d, want 0", count)
	}

	foundError := false
	for _, record := range loggertest.Records(t, logBuffer) {
		if record["msg"] == "trip created" {
			t.Fatal("success log must not be emitted on error")
		}

		if record["msg"] == "failed to create trip" {
			foundError = true

			if record["level"] != levelError {
				t.Fatalf("unexpected log level: %v", record["level"])
			}
		}
	}

	if !foundError {
		t.Fatal("expected failed to create trip log")
	}
}
