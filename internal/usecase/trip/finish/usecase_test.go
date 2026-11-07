package finish

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/usecase/trip/finish/mocks"
)

func TestFinishTripSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)

	tripRepo := mocks.NewMocktripRepository(ctrl)
	historyRepo := mocks.NewMockhistoryRepository(ctrl)
	txManager := mocks.NewMocktxManager(ctrl)

	id := uuid.New()

	expected := trip.Trip{
		ID:     id,
		Status: trip.TripStatusCompleted,
	}

	txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			fn func(context.Context) error,
		) error {
			return fn(ctx)
		})

	tripRepo.EXPECT().
		Finish(gomock.Any(), id).
		Return(expected, nil)

	historyRepo.EXPECT().
		Create(
			gomock.Any(),
			id,
			gomock.Any(),
			trip.TripStatusCompleted,
			gomock.Any(),
		).
		Return(nil)

	usecase := New(tripRepo, historyRepo, txManager)

	result, err := usecase.Execute(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != id {
		t.Fatalf("unexpected trip ID: %s", result.ID)
	}

	if result.Status != trip.TripStatusCompleted {
		t.Fatalf("unexpected status: %s", result.Status)
	}
}

func TestFinishTripAlreadyCompleted(t *testing.T) {
	ctrl := gomock.NewController(t)

	tripRepo := mocks.NewMocktripRepository(ctrl)
	historyRepo := mocks.NewMockhistoryRepository(ctrl)
	txManager := mocks.NewMocktxManager(ctrl)

	id := uuid.New()

	txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			fn func(context.Context) error,
		) error {
			return fn(ctx)
		})

	tripRepo.EXPECT().
		Finish(gomock.Any(), id).
		Return(trip.Trip{}, trip.ErrTripCompleted)

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

	_, err := usecase.Execute(context.Background(), id)

	if !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("expected ErrTripCompleted, got %v", err)
	}
}

func TestFinishTripHistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)

	tripRepo := mocks.NewMocktripRepository(ctrl)
	historyRepo := mocks.NewMockhistoryRepository(ctrl)
	txManager := mocks.NewMocktxManager(ctrl)

	id := uuid.New()
	expectedErr := errors.New("history insert failed")

	txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			fn func(context.Context) error,
		) error {
			return fn(ctx)
		})

	tripRepo.EXPECT().
		Finish(gomock.Any(), id).
		Return(trip.Trip{
			ID:     id,
			Status: trip.TripStatusCompleted,
		}, nil)

	historyRepo.EXPECT().
		Create(
			gomock.Any(),
			id,
			gomock.Any(),
			trip.TripStatusCompleted,
			gomock.Any(),
		).
		Return(expectedErr)

	usecase := New(tripRepo, historyRepo, txManager)

	_, err := usecase.Execute(context.Background(), id)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestFinishTripCommitError(t *testing.T) {
	ctrl := gomock.NewController(t)

	tripRepo := mocks.NewMocktripRepository(ctrl)
	historyRepo := mocks.NewMockhistoryRepository(ctrl)
	txManager := mocks.NewMocktxManager(ctrl)

	id := uuid.New()
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
		Finish(gomock.Any(), id).
		Return(trip.Trip{
			ID:     id,
			Status: trip.TripStatusCompleted,
		}, nil)

	historyRepo.EXPECT().
		Create(
			gomock.Any(),
			id,
			gomock.Any(),
			trip.TripStatusCompleted,
			gomock.Any(),
		).
		Return(nil)

	usecase := New(tripRepo, historyRepo, txManager)

	_, err := usecase.Execute(context.Background(), id)

	if !errors.Is(err, commitErr) {
		t.Fatalf("expected commit error, got %v", err)
	}
}
