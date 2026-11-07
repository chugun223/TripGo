package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type databasePingerStub struct {
	err    error
	called bool
}

func (s *databasePingerStub) Ping(_ context.Context) error {
	s.called = true
	return s.err
}

func TestReadySuccess(t *testing.T) {
	pinger := &databasePingerStub{}

	handler := NewHandler(
		nil,
		nil,
		nil,
		pinger,
		time.Second,
	)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.Ready(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if !pinger.called {
		t.Fatal("database Ping was not called")
	}

	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestReadyDatabaseUnavailable(t *testing.T) {
	pinger := &databasePingerStub{
		err: errors.New("database unavailable"),
	}

	handler := NewHandler(
		nil,
		nil,
		nil,
		pinger,
		time.Second,
	)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.Ready(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}

	if !pinger.called {
		t.Fatal("database Ping was not called")
	}

	if !strings.Contains(rec.Body.String(), `"status":"unavailable"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}
