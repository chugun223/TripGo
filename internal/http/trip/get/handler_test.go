package get

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/http/trip/get/mocks"
)

func TestGetTripSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)

	getter := mocks.NewMocktripGetter(ctrl)

	id := uuid.New()

	getter.EXPECT().
		Execute(gomock.Any(), id).
		Return(trip.Trip{
			ID:     id,
			Status: trip.TripStatusActive,
		}, nil)

	handler := New(getter)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/trips/"+id.String(),
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.Handle(recorder, request, id)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected status 200, got %d",
			recorder.Code,
		)
	}
}

func TestGetTripNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	getter := mocks.NewMocktripGetter(ctrl)

	id := uuid.New()

	getter.EXPECT().
		Execute(gomock.Any(), id).
		Return(trip.Trip{}, trip.ErrTripNotFound)

	handler := New(getter)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/trips/"+id.String(),
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.Handle(recorder, request, id)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected status 404, got %d",
			recorder.Code,
		)
	}

	if recorder.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatal("unexpected Content-Type")
	}
}
