package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const traceBatchTimeout = 2 * time.Second

func InitTracing(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"create OTLP trace exporter: %w",
			err,
		)
	}

	res, err := NewResource(ctx)
	if err != nil {
		if shutdownErr := exporter.Shutdown(ctx); shutdownErr != nil {
			return nil, errors.Join(err, shutdownErr)
		}
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exporter,
			sdktrace.WithBatchTimeout(traceBatchTimeout),
		),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(provider)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return provider.Shutdown, nil
}
