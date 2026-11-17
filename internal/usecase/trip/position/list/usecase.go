package list

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
)

type Usecase struct {
	trips     tripRepository
	positions positionRepository
	timeout   time.Duration
}

func New(trips tripRepository, positions positionRepository, timeout time.Duration) *Usecase {
	return &Usecase{trips: trips, positions: positions, timeout: timeout}
}

func (u *Usecase) Execute(ctx context.Context, id uuid.UUID) ([]trip.Position, error) {
	if id == uuid.Nil {
		return nil, &trip.ValidationError{Field: "tripId", Message: "must be a nonzero UUID"}
	}
	ctx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()
	if _, err := u.trips.GetByID(ctx, id); err != nil {
		return nil, fmt.Errorf("get trip for positions: %w", err)
	}
	positions, err := u.positions.List(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list trip positions: %w", err)
	}
	return positions, nil
}
