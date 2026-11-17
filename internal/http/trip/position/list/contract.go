package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=list_test

type positionLister interface {
	Execute(ctx context.Context, id uuid.UUID) ([]trip.Position, error)
}
