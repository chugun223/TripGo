package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

func New(level string) *slog.Logger {
	var logLevel slog.Level

	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{
			Level: logLevel,
		},
	)

	return slog.New(handler).With(
		slog.String("service", "trip-service"),
	)
}

func WithTrace(
	ctx context.Context,
	logger *slog.Logger,
) *slog.Logger {
	spanContext := trace.SpanContextFromContext(ctx)

	if !spanContext.IsValid() {
		return logger
	}

	return logger.With(
		slog.String("trace_id", spanContext.TraceID().String()),
		slog.String("span_id", spanContext.SpanID().String()),
	)
}
