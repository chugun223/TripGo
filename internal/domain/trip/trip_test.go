package trip

import (
	"errors"
	"testing"
	"time"
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

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Finish() error = %v, want %v", err, tt.wantErr)
			}

			if trip.Status != tt.wantStatus {
				t.Errorf(
					"Finish() status = %s, want %s",
					trip.Status,
					tt.wantStatus,
				)
			}

			if tt.wantErr == nil {
				if trip.FinishedAt == nil {
					t.Fatal("FinishedAt must be set")
				}

				if !trip.FinishedAt.Equal(finishedAt) {
					t.Errorf("FinishedAt = %v, want %v",
						*trip.FinishedAt, finishedAt)
				}
			} else if trip.FinishedAt != nil {
				t.Errorf("FinishedAt must remain nil on error")
			}
		})
	}
}
