package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=create_test

type tripRepository interface {
	LockActive(ctx context.Context, id uuid.UUID) error
	UpdateLastPosition(ctx context.Context, id uuid.UUID, recordedAt time.Time) error
}

type positionRepository interface {
	Create(ctx context.Context, position trip.Position) (trip.Position, error)
}

type txManager interface {
	Do(ctx context.Context, fn func(context.Context) error) error
}

type positionQueue interface {
	Cancel(id uuid.UUID)
}
