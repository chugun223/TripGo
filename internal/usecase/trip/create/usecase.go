package create

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/model"
)

type Usecase struct {
	tripRepository    tripRepository
	historyRepository historyRepository
	txManager         txManager
}

func New(
	tripRepository tripRepository,
	historyRepository historyRepository,
	txManager txManager,
) *Usecase {
	return &Usecase{
		tripRepository:    tripRepository,
		historyRepository: historyRepository,
		txManager:         txManager,
	}
}

func (u *Usecase) Execute(
	ctx context.Context,
	input model.CreateTripInput,
) (trip.Trip, error) {
	newTrip := trip.Trip{
		ID:         uuid.New(),
		UserID:     input.UserID,
		DriverID:   input.DriverID,
		StartPoint: input.StartPoint,
		EndPoint:   input.EndPoint,
		Price:      input.Price,
	}

	var createdTrip trip.Trip

	err := u.txManager.Do(ctx, func(txCtx context.Context) error {
		var err error

		createdTrip, err = u.tripRepository.Create(txCtx, newTrip)
		if err != nil {
			return fmt.Errorf("create trip: %w", err)
		}

		reason := "trip created"

		err = u.historyRepository.Create(
			txCtx,
			createdTrip.ID,
			nil,
			createdTrip.Status,
			&reason,
		)
		if err != nil {
			return fmt.Errorf("record initial trip status: %w", err)
		}

		return nil
	})
	if err != nil {
		return trip.Trip{}, err
	}

	return createdTrip, nil
}
