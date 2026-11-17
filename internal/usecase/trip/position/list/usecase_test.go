package list_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/usecase/trip/position/list"
)

func TestListPositions(t *testing.T) {
	for _, tc := range []struct {
		name             string
		tripErr, listErr error
	}{
		{name: "completed trip route is readable"},
		{name: "missing trip", tripErr: trip.ErrTripNotFound},
		{name: "repository error", listErr: errors.New("database unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			trips, positions := NewMocktripRepository(ctrl), NewMockpositionRepository(ctrl)
			id := uuid.New()
			trips.EXPECT().GetByID(gomock.Any(), id).Return(trip.Trip{Status: trip.TripStatusCompleted}, tc.tripErr)
			want := []trip.Position{{ID: 1, TripID: id}}
			if tc.tripErr == nil {
				positions.EXPECT().List(gomock.Any(), id).Return(want, tc.listErr)
			}
			uc := list.New(trips, positions, time.Second)
			result, err := uc.Execute(context.Background(), id)
			switch {
			case tc.tripErr != nil:
				require.ErrorIs(t, err, tc.tripErr)
			case tc.listErr != nil:
				require.ErrorIs(t, err, tc.listErr)
			default:
				require.NoError(t, err)
				require.Equal(t, want, result)
			}
		})
	}
}

func TestEmptyTripID(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := list.New(NewMocktripRepository(ctrl), NewMockpositionRepository(ctrl), time.Second)
	_, err := uc.Execute(context.Background(), uuid.Nil)
	var validation *trip.ValidationError
	require.ErrorAs(t, err, &validation)
}
