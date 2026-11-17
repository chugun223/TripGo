package create

import (
	"context"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=create_test

type positionCreator interface {
	Execute(ctx context.Context, input model.CreatePositionInput) (trip.Position, error)
}
