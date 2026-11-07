package create

import (
	"context"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/model"
)

type tripCreator interface {
	Execute(
		ctx context.Context,
		input model.CreateTripInput,
	) (domain.Trip, error)
}
