package create_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/domain/trip"
	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/http/trip/position/create"
	"github.com/chugun223/TripGo/internal/model"
)

const validBody = `{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"2026-11-15T12:00:00Z"}`

func TestCreatePosition(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"created", nil, 201, ""},
		{"missing trip", trip.ErrTripNotFound, 404, "trip_not_found"},
		{"completed trip", trip.ErrTripCompleted, 409, "trip_completed"},
		{"future timestamp", &trip.ValidationError{Field: "recorded_at", Message: "in the future"}, 400, "invalid_request"},
		{"internal error", errors.New("secret database details"), 500, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockpositionCreator(ctrl)
			id := uuid.New()
			at := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
			uc.EXPECT().Execute(gomock.Any(), model.CreatePositionInput{TripID: id, RecordedAt: at}).
				Return(trip.Position{ID: 42, TripID: id, RecordedAt: at}, tc.err)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/positions", strings.NewReader(validBody))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			create.New(uc).Handle(recorder, request, id)
			require.Equal(t, tc.status, recorder.Code)
			require.NotContains(t, recorder.Body.String(), "secret database details")
			if tc.code != "" {
				var problem api.Problem
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &problem))
				require.Equal(t, tc.code, problem.Code)
				require.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))
			} else {
				var position api.TripPosition
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &position))
				require.Equal(t, int64(42), position.Id)
				require.Equal(t, id, position.TripId)
				require.Equal(t, at, position.RecordedAt)
				require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
			}
		})
	}
}

func TestInvalidPositionJSON(t *testing.T) {
	for name, body := range map[string]string{
		"empty": "", "null": "null", "array": "[]", "missing fields": "{}",
		"missing latitude":  `{"coordinates":{"longitude":0},"recorded_at":"2026-11-15T12:00:00Z"}`,
		"null coordinate":   strings.Replace(validBody, `"latitude":0`, `"latitude": null `, 1),
		"null time":         strings.Replace(validBody, `"2026-11-15T12:00:00Z"`, "null", 1),
		"unknown field":     strings.TrimSuffix(validBody, "}") + `,"extra":true}`,
		"nested unknown":    strings.Replace(validBody, `"longitude":0`, `"longitude":0,"extra":1`, 1),
		"wrong case":        strings.Replace(validBody, "coordinates", "COORDINATES", 1),
		"extra JSON":        validBody + "{}",
		"invalid timestamp": strings.Replace(validBody, "2026-11-15T12:00:00Z", "yesterday", 1),
		"oversized":         strings.Repeat(" ", 1<<20) + validBody,
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			r := httptest.NewRequest(http.MethodPost, "/positions", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			create.New(NewMockpositionCreator(ctrl)).Handle(w, r, uuid.New())
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), `"code":"invalid_request"`)
		})
	}
}

func TestPositionContentType(t *testing.T) {
	ctrl := gomock.NewController(t)
	r := httptest.NewRequest(http.MethodPost, "/positions", strings.NewReader(validBody))
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	create.New(NewMockpositionCreator(ctrl)).Handle(w, r, uuid.New())
	require.Equal(t, http.StatusBadRequest, w.Code)
}
