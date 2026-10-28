package get

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/usecase/trip/get/mocks"
)

func TestGetTripSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)

	repo := mocks.NewMocktripRepository(ctrl)

	id := uuid.New()

	expected := trip.Trip{
		ID:     id,
		Status: trip.TripStatusActive,
	}

	repo.EXPECT().
		GetByID(gomock.Any(), id).
		Return(expected, nil).
		Times(1)

	usecase := New(repo)

	result, err := usecase.Execute(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != expected.ID {
		t.Fatalf("unexpected trip ID")
	}
}

func TestGetTripNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	repo := mocks.NewMocktripRepository(ctrl)

	id := uuid.New()

	repo.EXPECT().
		GetByID(gomock.Any(), id).
		Return(trip.Trip{}, trip.ErrTripNotFound).
		Times(1)

	usecase := New(repo)

	_, err := usecase.Execute(context.Background(), id)

	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("expected ErrTripNotFound, got %v", err)
	}
}
