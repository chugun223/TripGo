package finish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
)

type fakeFinisher struct {
	result domain.Trip
	err    error
}

func (f *fakeFinisher) Execute(
	_ context.Context,
	_ uuid.UUID,
) (domain.Trip, error) {
	return f.result, f.err
}

func TestFinishTripSuccess(t *testing.T) {
	id := uuid.New()

	handler := New(&fakeFinisher{
		result: domain.Trip{
			ID:     id,
			Status: domain.TripStatusCompleted,
		},
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/trips/"+id.String()+"/finish",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.Handle(
		recorder,
		request,
		id,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d",
			recorder.Code,
			http.StatusOK,
		)
	}
}

func TestFinishTripAlreadyCompleted(t *testing.T) {
	id := uuid.New()

	handler := New(&fakeFinisher{
		err: domain.ErrTripCompleted,
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/trips/"+id.String()+"/finish",
		nil,
	)

	recorder := httptest.NewRecorder()

	handler.Handle(
		recorder,
		request,
		id,
	)

	if recorder.Code != http.StatusConflict {
		t.Fatalf(
			"status = %d, want %d",
			recorder.Code,
			http.StatusConflict,
		)
	}
}
