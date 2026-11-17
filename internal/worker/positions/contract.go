package positions

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=positions

type tripRepository interface {
	FindStaleBatch(ctx context.Context, staleAfter time.Duration, after *model.StaleTripCursor, limit int) ([]model.StaleTrip, error)
	IsStale(ctx context.Context, id uuid.UUID, staleAfter time.Duration) (bool, error)
}

type pushGateway interface {
	RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error
}

type ticker interface {
	Ticks() <-chan time.Time
	Stop()
}
