package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

func TestHTTPMetrics(t *testing.T) {
	ctx := context.Background()

	// Создаём отдельный MeterProvider для теста.
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

	router := chi.NewRouter()
	router.Use(Metrics(instruments, router))

	const route = "/api/v1/trips/{tripId}"

	router.Get(route, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/trips/550e8400-e29b-41d4-a716-446655440000",
		nil,
	)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf(
			"expected HTTP status %d, got %d",
			http.StatusAccepted,
			response.Code,
		)
	}

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &collected); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	var foundDuration bool
	var foundActive bool

	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "http.server.request.duration":
				foundDuration = true

				if m.Unit != "s" {
					t.Errorf("duration unit: expected s, got %q", m.Unit)
				}

				histogram, ok := m.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatal("duration metric is not a float64 histogram")
				}

				if len(histogram.DataPoints) != 1 {
					t.Fatalf(
						"expected 1 duration data point, got %d",
						len(histogram.DataPoints),
					)
				}

				point := histogram.DataPoints[0]

				if point.Count != 1 {
					t.Errorf("expected duration count 1, got %d", point.Count)
				}

				if point.Sum < 0 {
					t.Errorf("duration must not be negative: %f", point.Sum)
				}

				checkStringAttribute(
					t,
					point.Attributes,
					"http.route",
					route,
				)

				checkStringAttribute(
					t,
					point.Attributes,
					"http.request.method",
					http.MethodGet,
				)

				checkIntAttribute(
					t,
					point.Attributes,
					"http.response.status_code",
					http.StatusAccepted,
				)

			case "http.server.active_requests":
				foundActive = true

				if m.Unit != "{request}" {
					t.Errorf(
						"active requests unit: expected {request}, got %q",
						m.Unit,
					)
				}

				counter, ok := m.Data.(metricdata.Sum[int64])
				if !ok {
					t.Fatal("active requests metric is not an int64 sum")
				}

				if len(counter.DataPoints) != 1 {
					t.Fatalf(
						"expected 1 active requests data point, got %d",
						len(counter.DataPoints),
					)
				}

				point := counter.DataPoints[0]

				if point.Value != 0 {
					t.Errorf(
						"expected 0 active requests after completion, got %d",
						point.Value,
					)
				}

				checkStringAttribute(
					t,
					point.Attributes,
					"http.route",
					route,
				)

				checkStringAttribute(
					t,
					point.Attributes,
					"http.request.method",
					http.MethodGet,
				)
			}
		}
	}

	if !foundDuration {
		t.Error("http.server.request.duration metric not found")
	}

	if !foundActive {
		t.Error("http.server.active_requests metric not found")
	}
}

func checkStringAttribute(
	t *testing.T,
	attrs attribute.Set,
	key string,
	expected string,
) {
	t.Helper()

	value, ok := attrs.Value(attribute.Key(key))
	if !ok {
		t.Errorf("attribute %q not found", key)
		return
	}

	if value.AsString() != expected {
		t.Errorf(
			"attribute %q: expected %q, got %q",
			key,
			expected,
			value.AsString(),
		)
	}
}

func checkIntAttribute(
	t *testing.T,
	attrs attribute.Set,
	key string,
	expected int,
) {
	t.Helper()

	value, ok := attrs.Value(attribute.Key(key))
	if !ok {
		t.Errorf("attribute %q not found", key)
		return
	}

	if value.AsInt64() != int64(expected) {
		t.Errorf(
			"attribute %q: expected %d, got %d",
			key,
			expected,
			value.AsInt64(),
		)
	}
}

func TestHTTPActiveRequestsDuringRequest(t *testing.T) {
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

	router := chi.NewRouter()
	router.Use(Metrics(instruments, router))

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	router.Get("/api/v1/trips/{tripId}", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		defer close(done)

		request := httptest.NewRequest(
			http.MethodGet,
			"/api/v1/trips/123",
			nil,
		)
		router.ServeHTTP(httptest.NewRecorder(), request)
	}()

	<-started

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &collected); err != nil {
		close(release)
		<-done
		t.Fatalf("collect metrics: %v", err)
	}

	found := false

	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "http.server.active_requests" {
				continue
			}

			found = true

			counter, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Error("active requests metric is not an int64 sum")
				continue
			}

			if len(counter.DataPoints) != 1 {
				t.Errorf("expected 1 data point, got %d", len(counter.DataPoints))
				continue
			}

			point := counter.DataPoints[0]

			if point.Value != 1 {
				t.Errorf("expected 1 active request, got %d", point.Value)
			}

			checkStringAttribute(
				t,
				point.Attributes,
				"http.route",
				"/api/v1/trips/{tripId}",
			)
		}
	}

	close(release)
	<-done

	if !found {
		t.Error("http.server.active_requests metric not found")
	}
}
