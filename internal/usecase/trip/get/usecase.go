package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
)

type Usecase struct {
	tripRepository tripRepository
}

func New(tripRepository tripRepository) *Usecase {
	return &Usecase{
		tripRepository: tripRepository,
	}
}

func (u *Usecase) Execute(
	ctx context.Context,
	id uuid.UUID,
) (trip.Trip, error) {
	result, err := u.tripRepository.GetByID(ctx, id)
	if err != nil {
		return trip.Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return result, nil
}
