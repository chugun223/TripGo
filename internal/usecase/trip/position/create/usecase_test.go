package create_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/model"
	"github.com/chugun223/TripGo/internal/usecase/trip/position/create"
)

func TestCreatePositionTransaction(t *testing.T) {
	const (
		insertFailure = "insert"
		lockFailure   = "lock"
		updateFailure = "update"
		commitFailure = "commit"
	)
	for _, failure := range []string{"", lockFailure, insertFailure, updateFailure, commitFailure} {
		t.Run("failure_"+failure, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			trips := NewMocktripRepository(ctrl)
			positions := NewMockpositionRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			queue := NewMockpositionQueue(ctrl)
			now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
			input := model.CreatePositionInput{TripID: uuid.New(), RecordedAt: now, Coordinates: trip.Coordinates{Latitude: 56, Longitude: 60}}
			failureErr := errors.New("database operation failed")
			committed := false
			type transactionKey struct{}
			tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				_, hasDeadline := ctx.Deadline()
				require.True(t, hasDeadline)
				txCtx := context.WithValue(ctx, transactionKey{}, true)
				lockErr := error(nil)
				if failure == lockFailure {
					lockErr = trip.ErrTripCompleted
				}
				trips.EXPECT().LockActive(txCtx, input.TripID).Return(lockErr)
				if failure != lockFailure {
					position := trip.Position{TripID: input.TripID, Coordinates: input.Coordinates, RecordedAt: now}
					saved := position
					saved.ID = 42
					insertErr := error(nil)
					if failure == insertFailure {
						insertErr = failureErr
					}
					positions.EXPECT().Create(txCtx, position).Return(saved, insertErr)
					if failure != insertFailure {
						updateErr := error(nil)
						if failure == updateFailure {
							updateErr = failureErr
						}
						trips.EXPECT().UpdateLastPosition(txCtx, input.TripID, now).Return(updateErr)
					}
				}
				if err := fn(txCtx); err != nil {
					return err
				}
				if failure == commitFailure {
					return failureErr
				}
				committed = true
				return nil
			})
			if failure == "" {
				queue.EXPECT().Cancel(input.TripID).Do(func(uuid.UUID) { require.True(t, committed, "cancel must happen after commit") })
			}
			uc := create.New(trips, positions, tx, queue, func() time.Time { return now }, time.Second)
			result, err := uc.Execute(context.Background(), input)
			if failure == "" {
				require.NoError(t, err)
				require.Equal(t, int64(42), result.ID)
				require.Equal(t, input.TripID, result.TripID)
			} else {
				require.Error(t, err)
				require.Equal(t, trip.Position{}, result)
				if failure == lockFailure {
					require.ErrorIs(t, err, trip.ErrTripCompleted)
				} else {
					require.ErrorIs(t, err, failureErr)
				}
			}
		})
	}
}

func TestInvalidPositionDoesNotOpenTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := create.New(NewMocktripRepository(ctrl), NewMockpositionRepository(ctrl), NewMocktxManager(ctrl),
		NewMockpositionQueue(ctrl), time.Now, time.Second)
	_, err := uc.Execute(context.Background(), model.CreatePositionInput{})
	var validation *trip.ValidationError
	require.ErrorAs(t, err, &validation)
}
