package observability_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/observability"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
	"github.com/chugun223/TripGo/internal/infra/telemetry/telemetrytest"
	"github.com/chugun223/TripGo/internal/model"
)

func TestSavedPositionMetricOnlyAfterSuccess(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) })
	metrics, err := telemetry.NewPositionMetrics()
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	next := NewMockPositionCreator(ctrl)
	input := model.CreatePositionInput{TripID: uuid.New()}
	failed := errors.New("commit failed")
	gomock.InOrder(
		next.EXPECT().Execute(gomock.Any(), input).Return(trip.Position{}, failed),
		next.EXPECT().Execute(gomock.Any(), input).Return(trip.Position{ID: 42}, nil),
	)
	observed := observability.NewCreatePosition(next, metrics, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err = observed.Execute(context.Background(), input)
	require.ErrorIs(t, err, failed)
	require.Zero(t, telemetrytest.CounterValue(t, reader, "tripgo.positions.saved"))
	_, err = observed.Execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, int64(1), telemetrytest.CounterValue(t, reader, "tripgo.positions.saved"))
}
