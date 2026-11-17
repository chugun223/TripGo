package list_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/http/trip/position/list"
)

func TestListPositions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		positions []trip.Position
		err       error
		status    int
	}{
		{"empty route", nil, nil, 200},
		{"route", []trip.Position{{ID: 42}}, nil, 200},
		{"missing trip", nil, trip.ErrTripNotFound, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockpositionLister(ctrl)
			id := uuid.New()
			uc.EXPECT().Execute(gomock.Any(), id).Return(tc.positions, tc.err)
			w := httptest.NewRecorder()
			list.New(uc).Handle(w, httptest.NewRequest(http.MethodGet, "/positions", nil), id)
			require.Equal(t, tc.status, w.Code)
			if tc.err == nil && len(tc.positions) == 0 {
				require.JSONEq(t, `{"positions":[]}`, w.Body.String())
			}
			if len(tc.positions) > 0 {
				require.Contains(t, w.Body.String(), `"id":42`)
			}
		})
	}
}
