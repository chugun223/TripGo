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
)

const tripCompletedLogMsg = "trip completed"

type finishExecutorStub struct {
	result trip.Trip
	err    error
	calls  int
}

func (s *finishExecutorStub) Execute(
	_ context.Context,
	_ uuid.UUID,
) (trip.Trip, error) {
	s.calls++
	return s.result, s.err
}

func TestFinishSuccess(t *testing.T) {
	logBuffer := loggertest.Capture(t)
	metrics, reader := telemetrytest.NewTripMetrics(t)

	id := uuid.New()
	next := &finishExecutorStub{
		result: trip.Trip{
			ID:     id,
			Status: trip.TripStatusCompleted,
		},
	}

	decorator := NewFinish(next, metrics, slog.Default())

	result, err := decorator.Execute(context.Background(), id)
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
		"tripgo.trips.completed",
	)
	if count != 1 {
		t.Fatalf("completed trips counter = %d, want 1", count)
	}

	found := false
	for _, record := range loggertest.Records(t, logBuffer) {
		if record["msg"] != tripCompletedLogMsg {
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
		t.Fatal("expected trip completed log")
	}
}

func TestFinishAlreadyCompleted(t *testing.T) {
	logBuffer := loggertest.Capture(t)
	metrics, reader := telemetrytest.NewTripMetrics(t)

	id := uuid.New()
	next := &finishExecutorStub{
		err: trip.ErrTripCompleted,
	}

	decorator := NewFinish(next, metrics, slog.Default())

	_, err := decorator.Execute(context.Background(), id)

	if !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("expected ErrTripCompleted, got %v", err)
	}

	count := telemetrytest.CounterValue(
		t,
		reader,
		"tripgo.trips.completed",
	)
	if count != 0 {
		t.Fatalf("completed trips counter = %d, want 0", count)
	}

	for _, record := range loggertest.Records(t, logBuffer) {
		if record["msg"] == tripCompletedLogMsg {
			t.Fatal("success log must not be emitted")
		}

		if record["level"] == levelError {
			t.Fatal("business error must not be logged as ERROR")
		}
	}
}

func TestFinishErrorDoesNotIncrementMetric(t *testing.T) {
	logBuffer := loggertest.Capture(t)
	metrics, reader := telemetrytest.NewTripMetrics(t)

	id := uuid.New()
	expectedErr := errors.New("database unavailable")

	next := &finishExecutorStub{err: expectedErr}
	decorator := NewFinish(next, metrics, slog.Default())

	_, err := decorator.Execute(context.Background(), id)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	count := telemetrytest.CounterValue(
		t,
		reader,
		"tripgo.trips.completed",
	)
	if count != 0 {
		t.Fatalf("completed trips counter = %d, want 0", count)
	}

	foundError := false
	for _, record := range loggertest.Records(t, logBuffer) {
		if record["msg"] == tripCompletedLogMsg {
			t.Fatal("success log must not be emitted on error")
		}

		if record["msg"] == "failed to finish trip" {
			foundError = true

			if record["level"] != levelError {
				t.Fatalf("unexpected log level: %v", record["level"])
			}

			if record["trip_id"] != id.String() {
				t.Fatalf("unexpected trip_id: %v", record["trip_id"])
			}
		}
	}

	if !foundError {
		t.Fatal("expected failed to finish trip log")
	}
}
