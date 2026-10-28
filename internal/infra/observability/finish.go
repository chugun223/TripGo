package observability

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/logger"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

type FinishExecutor interface {
	Execute(context.Context, uuid.UUID) (trip.Trip, error)
}

type Finish struct {
	next    FinishExecutor
	metrics *telemetry.TripMetrics
	log     *slog.Logger
}

func NewFinish(
	next FinishExecutor,
	metrics *telemetry.TripMetrics,
	log *slog.Logger,
) *Finish {
	return &Finish{next: next, metrics: metrics, log: log}
}

func (o *Finish) Execute(
	ctx context.Context,
	id uuid.UUID,
) (trip.Trip, error) {
	result, err := o.next.Execute(ctx, id)
	if err != nil {
		if !errors.Is(err, trip.ErrTripCompleted) {
			logger.WithTrace(ctx, o.log).ErrorContext(
				ctx,
				"failed to finish trip",
				slog.String("trip_id", id.String()),
				slog.Any("error", err),
			)
		}
		return trip.Trip{}, err
	}

	o.metrics.Completed.Add(ctx, 1)

	logger.WithTrace(ctx, o.log).InfoContext(
		ctx,
		"trip completed",
		slog.String("trip_id", result.ID.String()),
	)

	return result, nil
}
