package observability

import (
	"context"
	"log/slog"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/logger"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
	"github.com/chugun223/TripGo/internal/model"
)

type CreateExecutor interface {
	Execute(context.Context, model.CreateTripInput) (trip.Trip, error)
}

type Create struct {
	next    CreateExecutor
	metrics *telemetry.TripMetrics
	log     *slog.Logger
}

func NewCreate(
	next CreateExecutor,
	metrics *telemetry.TripMetrics,
	log *slog.Logger,
) *Create {
	return &Create{next: next, metrics: metrics, log: log}
}

func (o *Create) Execute(
	ctx context.Context,
	input model.CreateTripInput,
) (trip.Trip, error) {
	result, err := o.next.Execute(ctx, input)
	if err != nil {
		logger.WithTrace(ctx, o.log).ErrorContext(
			ctx,
			"failed to create trip",
			slog.Any("error", err),
		)
		return trip.Trip{}, err
	}

	o.metrics.Created.Add(ctx, 1)

	logger.WithTrace(ctx, o.log).InfoContext(
		ctx,
		"trip created",
		slog.String("trip_id", result.ID.String()),
	)

	return result, nil
}
