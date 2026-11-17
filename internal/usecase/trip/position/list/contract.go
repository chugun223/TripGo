package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=list_test

type tripRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}

type positionRepository interface {
	List(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error)
}
