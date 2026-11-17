package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const metricExportInterval = 10 * time.Second

func InitMetrics(ctx context.Context) (func(context.Context) error, error) {
	exporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"create OTLP metric exporter: %w",
			err,
		)
	}

	res, err := NewResource(ctx)
	if err != nil {
		if shutdownErr := exporter.Shutdown(ctx); shutdownErr != nil {
			return nil, fmt.Errorf(
				"create metric resource: %w; shutdown exporter: %v",
				err,
				shutdownErr,
			)
		}

		return nil, err
	}

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(metricExportInterval),
	)

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)

	otel.SetMeterProvider(provider)

	return provider.Shutdown, nil
}
