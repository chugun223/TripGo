package domainerror

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/http/statuscode"
)

func TestProblem(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "trip not found",
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name:       "trip completed",
			err:        trip.ErrTripCompleted,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
		},
		{
			name:       "driver busy",
			err:        trip.ErrDriverBusy,
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
		},
		{
			name: "wrapped error",
			err: fmt.Errorf(
				"get trip: %w",
				trip.ErrTripNotFound,
			),
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name:       "internal error",
			err:        errors.New("database connection failed"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, problem := Problem(tt.err)

			if status != tt.wantStatus {
				t.Errorf(
					"status = %d, want %d",
					status,
					tt.wantStatus,
				)
			}

			if problem.Code != tt.wantCode {
				t.Errorf(
					"code = %s, want %s",
					problem.Code,
					tt.wantCode,
				)
			}

			if problem.Status != statuscode.ToInt32(tt.wantStatus) {
				t.Errorf(
					"problem status = %d, want %d",
					problem.Status,
					tt.wantStatus,
				)
			}
		})
	}
}
