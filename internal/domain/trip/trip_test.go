package trip

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTripFinish(t *testing.T) {
	finishedAt := time.Now().UTC()

	tests := []struct {
		name       string
		status     TripStatus
		wantStatus TripStatus
		wantErr    error
	}{
		{
			name:       "active trip can be finished",
			status:     TripStatusActive,
			wantStatus: TripStatusCompleted,
			wantErr:    nil,
		},
		{
			name:       "completed trip cannot be finished",
			status:     TripStatusCompleted,
			wantStatus: TripStatusCompleted,
			wantErr:    ErrTripCompleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trip := Trip{
				Status: tt.status,
			}

			err := trip.Finish(finishedAt)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantStatus, trip.Status)

			if tt.wantErr == nil {
				require.NotNil(t, trip.FinishedAt)
				assert.True(
					t,
					trip.FinishedAt.Equal(finishedAt),
				)
			} else {
				assert.Nil(t, trip.FinishedAt)
			}
		})
	}
}
