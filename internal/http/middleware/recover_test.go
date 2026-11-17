package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverHandlesPanic(t *testing.T) {
	var logs bytes.Buffer

	log := slog.New(slog.NewJSONHandler(
		&logs,
		nil,
	))

	handler := Recover(log)(
		http.HandlerFunc(func(
			_ http.ResponseWriter,
			_ *http.Request,
		) {
			panic("test panic")
		}),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/test",
		nil,
	)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusInternalServerError,
			recorder.Code,
		)
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("unexpected Content-Type: %s", contentType)
	}

	var problem struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem response: %v", err)
	}

	if problem.Status != http.StatusInternalServerError {
		t.Errorf("expected problem status 500, got %d", problem.Status)
	}

	if problem.Code != "internal_error" {
		t.Errorf("expected internal_error, got %s", problem.Code)
	}

	if !strings.Contains(logs.String(), "HTTP handler panic") {
		t.Error("expected panic message in logs")
	}

	if !strings.Contains(logs.String(), "test panic") {
		t.Error("expected panic value in logs")
	}

	if !strings.Contains(logs.String(), "stack") {
		t.Error("expected stack trace in logs")
	}
}

func TestRecoverWithoutPanic(t *testing.T) {
	log := slog.New(slog.NewTextHandler(
		&bytes.Buffer{},
		nil,
	))

	handler := Recover(log)(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			_ *http.Request,
		) {
			w.WriteHeader(http.StatusAccepted)
		}),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/test",
		nil,
	)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusAccepted,
			recorder.Code,
		)
	}
}

func TestRecoverAfterResponseStarted(t *testing.T) {
	var logs bytes.Buffer

	log := slog.New(slog.NewJSONHandler(
		&logs,
		nil,
	))

	handler := Recover(log)(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			_ *http.Request,
		) {
			w.WriteHeader(http.StatusAccepted)
			panic("panic after response started")
		}),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/test",
		nil,
	)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusAccepted,
			recorder.Code,
		)
	}

	if !strings.Contains(logs.String(), "panic after response started") {
		t.Error("expected panic message in logs")
	}
}
