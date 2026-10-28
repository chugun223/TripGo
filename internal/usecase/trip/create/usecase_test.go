package create

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/model"
	"github.com/chugun223/TripGo/internal/usecase/trip/create/mocks"
)

func TestCreateTripSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)

	tripRepo := mocks.NewMocktripRepository(ctrl)
	historyRepo := mocks.NewMockhistoryRepository(ctrl)
	txManager := mocks.NewMocktxManager(ctrl)

	input := model.CreateTripInput{
		UserID:   uuid.New(),
		DriverID: uuid.New(),
		Price:    1000,
	}

	var createdTrip trip.Trip

	txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			fn func(context.Context) error,
		) error {
			return fn(ctx)
		})

	tripRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context,
			value trip.Trip,
		) (trip.Trip, error) {
			value.Status = trip.TripStatusActive
			createdTrip = value
			return value, nil
		})

	historyRepo.EXPECT().
		Create(
			gomock.Any(),
			gomock.Any(),
			gomock.Nil(),
			trip.TripStatusActive,
			gomock.Any(),
		).
		Return(nil)

	usecase := New(tripRepo, historyRepo, txManager)

	result, err := usecase.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID == uuid.Nil {
		t.Fatal("trip ID must not be empty")
	}

	if result.ID != createdTrip.ID {
		t.Fatal("unexpected trip ID")
	}

	if result.Status != trip.TripStatusActive {
		t.Fatalf("unexpected status: %s", result.Status)
	}

	if result.UserID != input.UserID {
		t.Fatal("unexpected user ID")
	}

	if result.DriverID != input.DriverID {
		t.Fatal("unexpected driver ID")
	}

	if result.Price != input.Price {
		t.Fatalf("unexpected price: %d", result.Price)
	}
}

func TestCreateTripRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)

	tripRepo := mocks.NewMocktripRepository(ctrl)
	historyRepo := mocks.NewMockhistoryRepository(ctrl)
	txManager := mocks.NewMocktxManager(ctrl)

	expectedErr := errors.New("database unavailable")

	txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			fn func(context.Context) error,
		) error {
			return fn(ctx)
		})

	tripRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(trip.Trip{}, expectedErr)

	historyRepo.EXPECT().
		Create(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).
		Times(0)

	usecase := New(tripRepo, historyRepo, txManager)

	_, err := usecase.Execute(
		context.Background(),
		model.CreateTripInput{
			UserID:   uuid.New(),
			DriverID: uuid.New(),
			Price:    1000,
		},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestCreateTripHistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)

	tripRepo := mocks.NewMocktripRepository(ctrl)
	historyRepo := mocks.NewMockhistoryRepository(ctrl)
	txManager := mocks.NewMocktxManager(ctrl)

	expectedErr := errors.New("history unavailable")

	txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			fn func(context.Context) error,
		) error {
			return fn(ctx)
		})

	tripRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context,
			value trip.Trip,
		) (trip.Trip, error) {
			value.Status = trip.TripStatusActive
			return value, nil
		})

	historyRepo.EXPECT().
		Create(
			gomock.Any(),
			gomock.Any(),
			gomock.Nil(),
			trip.TripStatusActive,
			gomock.Any(),
		).
		Return(expectedErr)

	usecase := New(tripRepo, historyRepo, txManager)

	_, err := usecase.Execute(
		context.Background(),
		model.CreateTripInput{
			UserID:   uuid.New(),
			DriverID: uuid.New(),
			Price:    1000,
		},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestCreateTripCommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	tripRepo := mocks.NewMocktripRepository(ctrl)
	historyRepo := mocks.NewMockhistoryRepository(ctrl)
	txManager := mocks.NewMocktxManager(ctrl)

	commitErr := errors.New("commit failed")

	txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			fn func(context.Context) error,
		) error {
			if err := fn(ctx); err != nil {
				return err
			}

			return commitErr
		})

	tripRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context,
			value trip.Trip,
		) (trip.Trip, error) {
			value.Status = trip.TripStatusActive
			return value, nil
		})

	historyRepo.EXPECT().
		Create(
			gomock.Any(),
			gomock.Any(),
			gomock.Nil(),
			trip.TripStatusActive,
			gomock.Any(),
		).
		Return(nil)

	usecase := New(tripRepo, historyRepo, txManager)

	_, err := usecase.Execute(
		context.Background(),
		model.CreateTripInput{
			UserID:   uuid.New(),
			DriverID: uuid.New(),
			Price:    1000,
		},
	)

	if !errors.Is(err, commitErr) {
		t.Fatalf("expected commit error, got %v", err)
	}
}
