package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
)

type tripRepository interface {
	Create(ctx context.Context, value trip.Trip) (trip.Trip, error)
}

type historyRepository interface {
	Create(
		ctx context.Context,
		tripID uuid.UUID,
		fromStatus *trip.TripStatus,
		toStatus trip.TripStatus,
		reason *string,
	) error
}

type txManager interface {
	Do(ctx context.Context, fn func(context.Context) error) error
}
