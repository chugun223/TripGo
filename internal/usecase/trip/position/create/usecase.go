package create

import (
	"context"
	"fmt"
	"time"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/model"
)

type Usecase struct {
	trips     tripRepository
	positions positionRepository
	tx        txManager
	queue     positionQueue
	now       func() time.Time
	timeout   time.Duration
}

func New(trips tripRepository, positions positionRepository, tx txManager,
	queue positionQueue, now func() time.Time, timeout time.Duration,
) *Usecase {
	return &Usecase{trips: trips, positions: positions, tx: tx, queue: queue, now: now, timeout: timeout}
}

func (u *Usecase) Execute(ctx context.Context, input model.CreatePositionInput) (trip.Position, error) {
	position := trip.Position{TripID: input.TripID, Coordinates: input.Coordinates, RecordedAt: input.RecordedAt.UTC()}
	if err := position.Validate(u.now()); err != nil {
		return trip.Position{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()
	var saved trip.Position
	err := u.tx.Do(ctx, func(txCtx context.Context) error {
		if err := u.trips.LockActive(txCtx, input.TripID); err != nil {
			return fmt.Errorf("check active trip: %w", err)
		}
		var err error
		saved, err = u.positions.Create(txCtx, position)
		if err != nil {
			return fmt.Errorf("save trip position: %w", err)
		}
		return u.trips.UpdateLastPosition(txCtx, input.TripID, position.RecordedAt)
	})
	if err != nil {
		return trip.Position{}, fmt.Errorf("create trip position: %w", err)
	}
	u.queue.Cancel(input.TripID)
	return saved, nil
}
