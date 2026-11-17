package positions

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/model"
)

func TestScan250TripsInThreeBatches(t *testing.T) {
	pool, repo, _, _, reader := newTestPool(t, 1, 300)
	trips := make([]model.StaleTrip, 250)
	for i := range trips {
		trips[i] = sampleTrip()
	}
	cursor := func(index int) *model.StaleTripCursor {
		return &model.StaleTripCursor{LastActivityAt: trips[index].LastActivityAt, ID: trips[index].ID}
	}
	gomock.InOrder(
		repo.EXPECT().FindStaleBatch(gomock.Any(), time.Minute, gomock.Nil(), 100).Return(trips[:100], nil),
		repo.EXPECT().FindStaleBatch(gomock.Any(), time.Minute, cursor(99), 100).Return(trips[100:200], nil),
		repo.EXPECT().FindStaleBatch(gomock.Any(), time.Minute, cursor(199), 100).Return(trips[200:], nil),
	)
	pool.scan(context.Background())
	require.Equal(t, int64(250), pool.QueueSize())
	seen := make(map[uuid.UUID]bool, len(trips))
	for _, want := range trips {
		queued := nextTask(t, pool)
		require.Equal(t, want, queued.trip)
		require.False(t, seen[queued.trip.ID])
		seen[queued.trip.ID] = true
		pool.finish(queued)
	}
	require.Len(t, seen, 250)
	data := collectMetrics(t, reader)
	var rows metricdata.Histogram[int64]
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == "tripgo.stale_checks.rows" {
				rows = metric.Data.(metricdata.Histogram[int64])
			}
		}
	}
	require.Len(t, rows.DataPoints, 1)
	require.Equal(t, uint64(1), rows.DataPoints[0].Count)
	require.Equal(t, int64(250), rows.DataPoints[0].Sum)
}

func TestFullQueueStopsPagingAndNextTickStartsOver(t *testing.T) {
	pool, repo, _, _, _ := newTestPool(t, 1, 1)
	trips := make([]model.StaleTrip, 100)
	for i := range trips {
		trips[i] = sampleTrip()
	}

	repo.EXPECT().FindStaleBatch(gomock.Any(), time.Minute, gomock.Nil(), 100).Return(trips, nil).Times(2)
	pool.scan(context.Background())
	pool.scan(context.Background())
	require.Equal(t, int64(1), pool.QueueSize())
	require.Equal(t, trips[0], nextTask(t, pool).trip)
}

func TestPriorityOrder(t *testing.T) {
	pool, repo, push, _, _ := newTestPool(t, 1, 3)
	old, middle, fresh := sampleTrip(), sampleTrip(), sampleTrip()
	for _, trip := range []model.StaleTrip{fresh, old, middle} {
		pool.enqueue(trip, trace.SpanContext{})
	}
	repo.EXPECT().IsStale(gomock.Any(), gomock.Any(), time.Minute).Return(true, nil).Times(3)
	gomock.InOrder(
		push.EXPECT().RequestPosition(gomock.Any(), old.DriverID, old.ID).Return(nil),
		push.EXPECT().RequestPosition(gomock.Any(), middle.DriverID, middle.ID).Return(nil),
		push.EXPECT().RequestPosition(gomock.Any(), fresh.DriverID, fresh.ID).Return(nil),
	)
	require.NoError(t, pool.Start(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, pool.Shutdown(ctx))
}

func TestPriorityTiesUseTripID(t *testing.T) {
	pool, _, _, _, _ := newTestPool(t, 1, 2)
	first, second := sampleTrip(), sampleTrip()
	first.ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	second.ID = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	second.LastActivityAt = first.LastActivityAt
	pool.enqueue(second, trace.SpanContext{})
	pool.enqueue(first, trace.SpanContext{})
	require.Equal(t, first.ID, nextTask(t, pool).trip.ID)
	require.Equal(t, second.ID, nextTask(t, pool).trip.ID)
}

func TestEvictionCancellationAndReplacement(t *testing.T) {
	pool, _, _, _, _ := newTestPool(t, 1, 2)
	old, middle, fresh := sampleTrip(), sampleTrip(), sampleTrip()
	pool.enqueue(fresh, trace.SpanContext{})
	pool.enqueue(middle, trace.SpanContext{})
	result, evicted := pool.enqueue(old, trace.SpanContext{})
	require.Equal(t, resultQueued, result)
	require.NotNil(t, evicted)
	require.Equal(t, fresh, evicted.trip)
	require.Equal(t, -1, evicted.index)
	require.NotContains(t, pool.pending, fresh.ID)
	require.Len(t, pool.pending, 2)
	pool.process(context.Background(), evicted)
	pool.Cancel(old.ID)
	require.Equal(t, int64(1), pool.QueueSize(), "cancel immediately frees heap capacity")
	require.Equal(t, resultQueued, enqueue(pool, fresh))
	require.Equal(t, resultDuplicate, enqueue(pool, fresh))
	require.Equal(t, middle, nextTask(t, pool).trip)
	require.Equal(t, fresh, nextTask(t, pool).trip)
}

func TestFullQueueRejectsFresherTrip(t *testing.T) {
	pool, _, _, _, _ := newTestPool(t, 1, 1)
	old, fresh := sampleTrip(), sampleTrip()
	pool.enqueue(old, trace.SpanContext{})
	result, evicted := pool.enqueue(fresh, trace.SpanContext{})
	require.Equal(t, resultDropped, result)
	require.Nil(t, evicted)
	require.Equal(t, old, nextTask(t, pool).trip)
}

func TestEvictedRequestIsCountedAsDropped(t *testing.T) {
	pool, repo, _, _, reader := newTestPool(t, 1, 2)
	old, middle, fresh := sampleTrip(), sampleTrip(), sampleTrip()
	repo.EXPECT().FindStaleBatch(gomock.Any(), time.Minute, gomock.Nil(), 100).
		Return([]model.StaleTrip{fresh, middle, old}, nil)
	pool.scan(context.Background())
	data := collectMetrics(t, reader)
	require.Equal(t, int64(3), sumValue(data, "tripgo.stale_checks", resultQueued))
	require.Equal(t, int64(1), sumValue(data, "tripgo.position_requests", resultDropped))
}

func TestEmptyPriorityQueueWakesOnCancellation(t *testing.T) {
	pool, _, _, _, _ := newTestPool(t, 4, 2)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, pool.Start(ctx))
	cancel()
	waitClosed(t, pool.done)
}
