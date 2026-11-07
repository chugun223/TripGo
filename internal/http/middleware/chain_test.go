package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

func TestMiddlewareChainPanic(t *testing.T) {
	ctx := context.Background()

	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)

	oldMeterProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(meterProvider)

	t.Cleanup(func() {
		otel.SetMeterProvider(oldMeterProvider)

		if err := meterProvider.Shutdown(ctx); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})

	traceExporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(traceExporter),
	)

	oldTracerProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tracerProvider)

	t.Cleanup(func() {
		otel.SetTracerProvider(oldTracerProvider)

		if err := tracerProvider.Shutdown(ctx); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})

	instruments, err := telemetry.NewHTTPMetrics()
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	var logs bytes.Buffer
	log := slog.New(
		slog.NewJSONHandler(&logs, nil),
	)

	router := chi.NewRouter()
	router.Use(RequestLogging(log))
	router.Use(Metrics(instruments, router))

	router.Get("/test", func(
		http.ResponseWriter,
		*http.Request,
	) {
		panic("test panic")
	})

	handler := otelhttp.NewHandler(
		Recover(log)(router),
		"HTTP request",
		otelhttp.WithMeterProvider(noop.NewMeterProvider()),
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)

	handler.ServeHTTP(recorder, request)

	assertHTTPResponse(t, recorder)

	spans := traceExporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]

	assertPanicSpan(t, span)
	assertPanicLogs(t, &logs, span)
	assertHTTPDurationMetric(t, ctx, reader)
}

func assertHTTPResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
) {
	t.Helper()

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
}

func assertPanicSpan(
	t *testing.T,
	span tracetest.SpanStub,
) {
	t.Helper()

	if span.Status.Code != codes.Error {
		t.Errorf("expected span status Error, got %v", span.Status.Code)
	}

	foundException := false

	for _, event := range span.Events {
		if event.Name == "exception" {
			foundException = true
			break
		}
	}

	if !foundException {
		t.Error("exception event not found in span")
	}
}

func assertPanicLogs(
	t *testing.T,
	logs *bytes.Buffer,
	span tracetest.SpanStub,
) {
	t.Helper()

	decoder := json.NewDecoder(logs)

	foundCompletion := false
	foundPanicLog := false

	for {
		var entry map[string]any

		err := decoder.Decode(&entry)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("decode log: %v", err)
		}

		switch entry["msg"] {
		case "http request completed":
			foundCompletion = true

			if entry["status"] != float64(500) {
				t.Errorf(
					"expected logged status 500, got %v",
					entry["status"],
				)
			}

			if entry["level"] != "ERROR" {
				t.Errorf(
					"expected ERROR level, got %v",
					entry["level"],
				)
			}

		case "HTTP handler panic":
			foundPanicLog = true

			if entry["trace_id"] != span.SpanContext.TraceID().String() {
				t.Errorf(
					"expected trace_id %s, got %v",
					span.SpanContext.TraceID(),
					entry["trace_id"],
				)
			}

			if entry["span_id"] != span.SpanContext.SpanID().String() {
				t.Errorf(
					"expected span_id %s, got %v",
					span.SpanContext.SpanID(),
					entry["span_id"],
				)
			}
		}
	}

	if !foundCompletion {
		t.Error("completion log not found")
	}

	if !foundPanicLog {
		t.Error("panic log not found")
	}
}

func assertHTTPDurationMetric(
	t *testing.T,
	ctx context.Context,
	reader *sdkmetric.ManualReader,
) {
	t.Helper()

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
