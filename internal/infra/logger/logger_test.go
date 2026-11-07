package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestWithTrace(t *testing.T) {
	var buffer bytes.Buffer

	log := slog.New(slog.NewJSONHandler(&buffer, nil))

	traceID, err := trace.TraceIDFromHex(
		"4bf92f3577b34da6a3ce929d0e0e4736",
	)
	if err != nil {
		t.Fatal(err)
	}

	spanID, err := trace.SpanIDFromHex(
		"00f067aa0ba902b7",
	)
	if err != nil {
		t.Fatal(err)
	}

	spanContext := trace.NewSpanContext(
		trace.SpanContextConfig{
			TraceID: traceID,
			SpanID:  spanID,
		},
	)

	ctx := trace.ContextWithSpanContext(
		context.Background(),
		spanContext,
	)

	WithTrace(ctx, log).InfoContext(ctx, "test log")

	var record map[string]any

	if err := json.Unmarshal(buffer.Bytes(), &record); err != nil {
		t.Fatal(err)
	}

	if record["trace_id"] != traceID.String() {
		t.Fatalf("unexpected trace_id: %v", record["trace_id"])
	}

	if record["span_id"] != spanID.String() {
		t.Fatalf("unexpected span_id: %v", record["span_id"])
	}
}
