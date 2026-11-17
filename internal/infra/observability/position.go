package observability

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/logger"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
	"github.com/chugun223/TripGo/internal/model"
)

//go:generate go tool mockgen -source=position.go -destination=position_mocks_test.go -package=observability_test

type PositionCreator interface {
	Execute(ctx context.Context, input model.CreatePositionInput) (trip.Position, error)
}

type PositionLister interface {
	Execute(ctx context.Context, id uuid.UUID) ([]trip.Position, error)
}

type CreatePosition struct {
	next    PositionCreator
	metrics *telemetry.PositionMetrics
	log     *slog.Logger
}

func NewCreatePosition(next PositionCreator, metrics *telemetry.PositionMetrics, log *slog.Logger) *CreatePosition {
	return &CreatePosition{next: next, metrics: metrics, log: log}
}

func (o *CreatePosition) Execute(ctx context.Context, input model.CreatePositionInput) (trip.Position, error) {
	position, err := o.next.Execute(ctx, input)
	if err != nil {
		logPositionError(ctx, o.log, err)
		return trip.Position{}, err
	}
	o.metrics.Saved.Add(ctx, 1)
	return position, nil
}

type ListPositions struct {
	next PositionLister
	log  *slog.Logger
}

func NewListPositions(next PositionLister, log *slog.Logger) *ListPositions {
	return &ListPositions{next: next, log: log}
}

func (o *ListPositions) Execute(ctx context.Context, id uuid.UUID) ([]trip.Position, error) {
	positions, err := o.next.Execute(ctx, id)
	if err != nil {
		logPositionError(ctx, o.log, err)
	}
	return positions, err
}

func logPositionError(ctx context.Context, log *slog.Logger, err error) {
	var validation *trip.ValidationError
	if !errors.Is(err, trip.ErrTripNotFound) && !errors.Is(err, trip.ErrTripCompleted) && !errors.As(err, &validation) {
		logger.WithTrace(ctx, log).ErrorContext(ctx, "trip position operation failed", slog.Any("error", err))
	}
}
