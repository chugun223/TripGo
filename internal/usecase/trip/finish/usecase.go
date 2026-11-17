package finish

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
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
	id uuid.UUID,
) (trip.Trip, error) {
	var finishedTrip trip.Trip

	err := u.txManager.Do(ctx, func(txCtx context.Context) error {
		var err error

		finishedTrip, err = u.tripRepository.Finish(txCtx, id)
		if err != nil {
			return fmt.Errorf("finish trip: %w", err)
		}

		fromStatus := trip.TripStatusActive
		toStatus := finishedTrip.Status
		reason := "trip finished"

		err = u.historyRepository.Create(
			txCtx,
			finishedTrip.ID,
			&fromStatus,
			toStatus,
			&reason,
		)
		if err != nil {
			return fmt.Errorf("record completed trip status: %w", err)
		}

		return nil
	})
	if err != nil {
		return trip.Trip{}, err
	}

	return finishedTrip, nil
}
