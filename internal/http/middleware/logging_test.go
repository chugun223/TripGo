package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

const (
	levelInfo  = "INFO"
	levelError = "ERROR"
)

func TestRequestLogging(t *testing.T) {
	var output bytes.Buffer

	log := slog.New(
		slog.NewJSONHandler(&output, nil),
	)

	router := chi.NewRouter()
	router.Use(RequestLogging(log))

	router.Get("/trips/{tripID}", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		w.WriteHeader(http.StatusOK)
	})

	request := httptest.NewRequest(
		http.MethodGet,
		"/trips/123",
		nil,
	)

	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected status 200, got %d",
			recorder.Code,
		)
	}

	var entry map[string]any

	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode log: %v", err)
	}

	if entry["msg"] != "http request completed" {
		t.Fatalf("unexpected log message: %v", entry["msg"])
	}

	if entry["method"] != http.MethodGet {
		t.Fatalf("unexpected method: %v", entry["method"])
	}

	if entry["route"] != "/trips/{tripID}" {
		t.Fatalf("unexpected route: %v", entry["route"])
	}

	if entry["status"] != float64(http.StatusOK) {
		t.Fatalf("unexpected status: %v", entry["status"])
	}

	if _, ok := entry["duration_ms"]; !ok {
		t.Fatal("duration_ms is missing")
	}
}

func TestRequestLoggingLevels(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		expectedLevel string
	}{
		{
			name:          "successful request",
			status:        http.StatusOK,
			expectedLevel: levelInfo,
		},
		{
			name:          "bad request",
			status:        http.StatusBadRequest,
			expectedLevel: levelInfo,
		},
		{
			name:          "not found",
			status:        http.StatusNotFound,
			expectedLevel: levelInfo,
		},
		{
			name:          "internal server error",
			status:        http.StatusInternalServerError,
			expectedLevel: levelError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer

			log := slog.New(
				slog.NewJSONHandler(&output, nil),
			)

			router := chi.NewRouter()
			router.Use(RequestLogging(log))

			router.Get("/test", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			})

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/test", nil)

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.status {
				t.Fatalf("expected status %d, got %d", tt.status, recorder.Code)
			}

			var entry map[string]any
			if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
				t.Fatalf("decode log: %v", err)
			}

			if entry["level"] != tt.expectedLevel {
				t.Errorf(
					"expected log level %s, got %v",
					tt.expectedLevel,
					entry["level"],
				)
			}

			if entry["status"] != float64(tt.status) {
				t.Errorf(
					"expected logged status %d, got %v",
					tt.status,
					entry["status"],
				)
			}
		})
	}
}
