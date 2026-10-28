package telemetry

import (
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type DBMetrics struct {
	OperationDuration metric.Float64Histogram
}

func NewDBMetrics() (*DBMetrics, error) {
	meter := otel.Meter("tripgo/postgres")

	operationDuration, err := meter.Float64Histogram(
		"db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription(
			"Duration of PostgreSQL database operations",
		),
		metric.WithExplicitBucketBoundaries(
			0.001,
			0.0025,
			0.005,
			0.01,
			0.025,
			0.05,
			0.1,
			0.25,
			0.5,
			1,
			2.5,
			5,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create database operation duration histogram: %w",
			err,
		)
	}

	return &DBMetrics{
		OperationDuration: operationDuration,
	}, nil
}
