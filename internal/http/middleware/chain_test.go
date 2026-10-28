package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

func TestMiddlewareChainPanic(t *testing.T) {
	ctx := context.Background()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)

	oldProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)

	t.Cleanup(func() {
		otel.SetMeterProvider(oldProvider)
		if err := provider.Shutdown(ctx); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})

	instruments, err := telemetry.NewHTTPMetrics()
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))

	router := chi.NewRouter()
	router.Use(RequestLogging(log))
	router.Use(Metrics(instruments, router))

	router.Get("/test", func(http.ResponseWriter, *http.Request) {
		panic("test panic")
	})

	handler := Recover(log)(router)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", recorder.Code)
	}

	var problem struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "internal_error" {
		t.Errorf("expected internal_error, got %q", problem.Code)
	}

	decoder := json.NewDecoder(&logs)
	foundCompletion := false

	for decoder.More() {
		var entry map[string]any
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("decode log: %v", err)
		}

		if entry["msg"] == "http request completed" {
			foundCompletion = true

			if entry["status"] != float64(500) {
				t.Errorf("expected logged status 500, got %v", entry["status"])
			}
			if entry["level"] != "ERROR" {
				t.Errorf("expected ERROR level, got %v", entry["level"])
			}
		}
	}

	if !foundCompletion {
		t.Error("completion log not found")
	}

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &collected); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	foundDuration := false

	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}

			histogram, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatal("duration is not a histogram")
			}

			for _, point := range histogram.DataPoints {
				status, ok := point.Attributes.Value(
					"http.response.status_code",
				)

				if ok && status.AsInt64() == 500 && point.Count == 1 {
					foundDuration = true
				}
			}
		}
	}

	if !foundDuration {
		t.Error("HTTP duration metric with status 500 not found")
	}
}
