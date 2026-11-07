package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
)

type tripRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}
