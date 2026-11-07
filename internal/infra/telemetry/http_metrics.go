package telemetry

import (
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type HTTPMetrics struct {
	RequestDuration metric.Float64Histogram
	ActiveRequests  metric.Int64UpDownCounter
}

func NewHTTPMetrics() (*HTTPMetrics, error) {
	meter := otel.Meter("tripgo/http")

	requestDuration, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests"),
		metric.WithExplicitBucketBoundaries(
			0.005,
			0.01,
			0.025,
			0.05,
			0.075,
			0.1,
			0.25,
			0.5,
			0.75,
			1,
			2.5,
			5,
			10,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create HTTP duration histogram: %w", err)
	}

	activeRequests, err := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Number of active HTTP server requests"),
	)
	if err != nil {
		return nil, fmt.Errorf("create active requests counter: %w", err)
	}

	return &HTTPMetrics{
		RequestDuration: requestDuration,
		ActiveRequests:  activeRequests,
	}, nil
}
