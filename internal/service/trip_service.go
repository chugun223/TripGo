package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/database"
	"github.com/chugun223/TripGo/internal/domain"
	"github.com/chugun223/TripGo/internal/repository"
)

type TripService struct {
	tripRepository    *repository.TripRepository
	historyRepository *repository.TripStatusHistoryRepository
	txManager         database.TxManager
}

func NewTripService(
	tripRepository *repository.TripRepository,
	historyRepository *repository.TripStatusHistoryRepository,
	txManager database.TxManager,
) *TripService {
	return &TripService{
		tripRepository:    tripRepository,
		historyRepository: historyRepository,
		txManager:         txManager,
	}
}

func (s *TripService) Create(
	ctx context.Context,
	trip domain.Trip,
) (domain.Trip, error) {
	if trip.ID == uuid.Nil {
		trip.ID = uuid.New()
	}

	var createdTrip domain.Trip

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		var err error

		createdTrip, err = s.tripRepository.Create(txCtx, trip)
		if err != nil {
			return fmt.Errorf("create trip: %w", err)
		}

		toStatus := createdTrip.Status
		reason := "trip created"

		err = s.historyRepository.Create(
			txCtx,
			createdTrip.ID,
			nil,
			toStatus,
			&reason,
		)
		if err != nil {
			return fmt.Errorf("record initial trip status: %w", err)
		}

		return nil
	})
	if err != nil {
		return domain.Trip{}, err
	}

	return createdTrip, nil
}

func (s *TripService) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (domain.Trip, error) {
	trip, err := s.tripRepository.GetByID(ctx, id)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return trip, nil
}

func (s *TripService) Finish(
	ctx context.Context,
	id uuid.UUID,
) (domain.Trip, error) {
	var finishedTrip domain.Trip

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		var err error

		finishedTrip, err = s.tripRepository.Finish(txCtx, id)
		if err != nil {
			return fmt.Errorf("finish trip: %w", err)
		}

		fromStatus := domain.TripStatusActive
		toStatus := finishedTrip.Status
		reason := "trip finished"

		err = s.historyRepository.Create(
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
		return domain.Trip{}, err
	}

	return finishedTrip, nil
}
