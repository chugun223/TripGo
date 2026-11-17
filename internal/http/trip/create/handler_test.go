package create

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/model"
)

type fakeCreator struct {
	called bool
	result domain.Trip
}

func (f *fakeCreator) Execute(
	_ context.Context,
	_ model.CreateTripInput,
) (domain.Trip, error) {
	f.called = true
	return f.result, nil
}

func TestCreateTripSuccess(t *testing.T) {
	id := uuid.New()

	creator := &fakeCreator{
		result: domain.Trip{
			ID:     id,
			Status: domain.TripStatusActive,
		},
	}

	handler := New(creator)

	body := `{
		"user_id": "550e8400-e29b-41d4-a716-446655440000",
		"driver_id": "550e8400-e29b-41d4-a716-446655440001",
		"start_point": {
			"latitude": 56.8389,
			"longitude": 60.6057
		},
		"end_point": {
			"latitude": 56.8500,
			"longitude": 60.6100
		},
		"price": 500
	}`

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/trips",
		strings.NewReader(body),
	)

	recorder := httptest.NewRecorder()

	handler.Handle(
		recorder,
		request,
		api.CreateTripParams{},
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"status = %d, want %d; body: %s",
			recorder.Code,
			http.StatusCreated,
			recorder.Body.String(),
		)
	}

	if !creator.called {
		t.Fatal("usecase was not called")
	}

	wantLocation := "/api/v1/trips/" + id.String()

	if recorder.Header().Get("Location") != wantLocation {
		t.Fatalf("unexpected Location header")
	}
}

func TestCreateTripInvalidRequest(t *testing.T) {
	creator := &fakeCreator{}
	handler := New(creator)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/trips",
		strings.NewReader(`{"price": -100}`),
	)

	recorder := httptest.NewRecorder()

	handler.Handle(
		recorder,
		request,
		api.CreateTripParams{},
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d",
			recorder.Code,
			http.StatusBadRequest,
		)
	}

	if creator.called {
		t.Fatal("usecase must not be called for invalid request")
	}
}
