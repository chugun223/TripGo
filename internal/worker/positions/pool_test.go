package positions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
	"github.com/chugun223/TripGo/internal/model"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type manualTicker struct {
	ticks   chan time.Time
	stopped chan struct{}
	once    sync.Once
}

func (t *manualTicker) Ticks() <-chan time.Time { return t.ticks }
func (t *manualTicker) Stop()                   { t.once.Do(func() { close(t.stopped) }) }

func newTestPool(t *testing.T, workers, capacity int) (*Pool, *MocktripRepository, *MockpushGateway, *manualTicker, *sdkmetric.ManualReader) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo, push := NewMocktripRepository(ctrl), NewMockpushGateway(ctrl)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	old := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	metrics, err := telemetry.NewPositionMetrics()
	require.NoError(t, err)
	pool, err := New(repo, push, Settings{
		Workers: workers, QueueSize: capacity, BatchSize: 100,
		CheckInterval: 30 * time.Second, StaleAfter: time.Minute, QueryTimeout: time.Second, RequestTimeout: time.Second,
	},
		metrics, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	ticks := &manualTicker{ticks: make(chan time.Time, 2), stopped: make(chan struct{})}
	pool.newTicker = func(interval time.Duration) ticker { require.Equal(t, 30*time.Second, interval); return ticks }
	require.NoError(t, metrics.RegisterPool(pool))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, pool.Shutdown(ctx))
		require.NoError(t, metrics.Close())
		otel.SetMeterProvider(old)
		require.NoError(t, provider.Shutdown(ctx))
	})
	return pool, repo, push, ticks, reader
}

func waitClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("operation did not complete")
	}
}

var sampleSequence atomic.Int64

func sampleTrip() model.StaleTrip {
	return model.StaleTrip{ID: uuid.New(), DriverID: uuid.New(), LastActivityAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(sampleSequence.Add(1)) * time.Second)}
}

func nextTask(t *testing.T, pool *Pool) *task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	value, ok := pool.take(ctx)
	require.True(t, ok)
	return value
}

func enqueue(pool *Pool, trip model.StaleTrip) string {
	result, _ := pool.enqueue(trip, trace.SpanContext{})
	return result
}

func TestCancellationAndReplacement(t *testing.T) {
	pool, repo, push, _, _ := newTestPool(t, 1, 2)
	trip := sampleTrip()
	require.Equal(t, resultQueued, enqueue(pool, trip))
	oldTask := nextTask(t, pool)
	require.Equal(t, resultDuplicate, enqueue(pool, trip))
	pool.Cancel(trip.ID)
	require.Equal(t, resultQueued, enqueue(pool, trip))
	pool.process(context.Background(), oldTask) // Canceled generation must never reach the gateway.
	pool.finish(oldTask)
	require.Contains(t, pool.pending, trip.ID, "an old task must not delete its replacement")
	repo.EXPECT().IsStale(gomock.Any(), trip.ID, time.Minute).Return(true, nil)
	push.EXPECT().RequestPosition(gomock.Any(), trip.DriverID, trip.ID).Return(nil)
	pool.process(context.Background(), nextTask(t, pool))
	require.Empty(t, pool.pending)
}

func TestCancelInFlightDoesNotDeleteReplacement(t *testing.T) {
	pool, repo, push, _, _ := newTestPool(t, 1, 2)
	trip := sampleTrip()
	pool.enqueue(trip, trace.SpanContext{})
	repo.EXPECT().IsStale(gomock.Any(), trip.ID, time.Minute).Return(true, nil).Times(2)
	first := push.EXPECT().RequestPosition(gomock.Any(), trip.DriverID, trip.ID).
		DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
			pool.Cancel(trip.ID)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Equal(t, resultQueued, enqueue(pool, trip))
			return ctx.Err()
		})
	push.EXPECT().RequestPosition(gomock.Any(), trip.DriverID, trip.ID).After(first).Return(nil)
	pool.process(context.Background(), nextTask(t, pool))
	require.Contains(t, pool.pending, trip.ID)
	pool.process(context.Background(), nextTask(t, pool))
	require.Empty(t, pool.pending)
}

func TestScannerDeduplicationAndOverflowMetrics(t *testing.T) {
	pool, repo, _, _, reader := newTestPool(t, 1, 1)
	trips := []model.StaleTrip{sampleTrip(), sampleTrip(), sampleTrip()}
	repo.EXPECT().FindStaleBatch(gomock.Any(), time.Minute, gomock.Nil(), 100).Return(trips, nil).Times(2)
	pool.scan(context.Background())
	pool.scan(context.Background())
	require.Equal(t, int64(1), pool.QueueSize())
	require.Len(t, pool.pending, 1)
	data := collectMetrics(t, reader)
	require.Equal(t, int64(1), sumValue(data, "tripgo.stale_checks", resultQueued))
	require.Equal(t, int64(1), sumValue(data, "tripgo.stale_checks", resultDuplicate))
	require.Equal(t, int64(4), sumValue(data, "tripgo.stale_checks", resultDropped))
	require.Equal(t, int64(4), sumValue(data, "tripgo.position_requests", resultDropped))
	require.Equal(t, int64(1), gaugeValue(data, "tripgo.position_requests.queue.size"))
	require.Equal(t, int64(0), gaugeValue(data, "tripgo.position_requests.workers.active"))
}

func TestWorkerLimitAndDrain(t *testing.T) {
	const workers, tasks = 2, 6
	pool, repo, push, ticks, reader := newTestPool(t, workers, tasks)
	for i := 0; i < tasks; i++ {
		pool.enqueue(sampleTrip(), trace.SpanContext{})
	}
	repo.EXPECT().IsStale(gomock.Any(), gomock.Any(), time.Minute).Return(true, nil).Times(tasks)
	started := make(chan struct{}, tasks)
	release := make(chan struct{}, tasks)
	var calls, running, peak atomic.Int64
	push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
			current := running.Add(1)
			for previous := peak.Load(); current > previous; previous = peak.Load() {
				if peak.CompareAndSwap(previous, current) {
					break
				}
			}
			defer running.Add(-1)
			calls.Add(1)
			started <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}).Times(tasks)
	require.NoError(t, pool.Start(context.Background()))
	for i := 0; i < workers; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not start")
		}
	}
	require.Equal(t, int64(workers), pool.ActiveWorkers())
	require.Equal(t, int64(tasks-workers), pool.QueueSize())
	data := collectMetrics(t, reader)
	require.Equal(t, int64(workers), gaugeValue(data, "tripgo.position_requests.workers.active"))
	shutdown := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdown <- pool.Shutdown(ctx)
	}()
	waitClosed(t, ticks.stopped)
	require.Equal(t, int64(workers), calls.Load(), "shutdown must not cancel current calls before draining")
	for i := 0; i < tasks; i++ {
		release <- struct{}{}
	}
	require.NoError(t, <-shutdown)
	require.Equal(t, int64(tasks), calls.Load())
	require.Equal(t, int64(workers), peak.Load())
	require.Zero(t, pool.ActiveWorkers())
	require.Zero(t, pool.QueueSize())
	data = collectMetrics(t, reader)
	require.Equal(t, int64(tasks), sumValue(data, "tripgo.position_requests", "success"))
}

func TestPushErrorDoesNotStopWorker(t *testing.T) {
	pool, repo, push, _, reader := newTestPool(t, 1, 2)
	first, second := sampleTrip(), sampleTrip()
	pool.enqueue(first, trace.SpanContext{})
	pool.enqueue(second, trace.SpanContext{})
	repo.EXPECT().IsStale(gomock.Any(), gomock.Any(), time.Minute).Return(true, nil).Times(2)
	gomock.InOrder(
		push.EXPECT().RequestPosition(gomock.Any(), first.DriverID, first.ID).Return(errors.New("push unavailable")),
		push.EXPECT().RequestPosition(gomock.Any(), second.DriverID, second.ID).Return(nil),
	)
	require.NoError(t, pool.Start(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, pool.Shutdown(ctx))
	data := collectMetrics(t, reader)
	require.Equal(t, int64(1), sumValue(data, "tripgo.position_requests", "error"))
	require.Equal(t, int64(1), sumValue(data, "tripgo.position_requests", "success"))
}

func TestShutdownDeadlineAndParentCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent cancellation", true: "shutdown deadline"}[deadline], func(t *testing.T) {
			pool, repo, push, ticks, _ := newTestPool(t, 1, 2)
			pool.enqueue(sampleTrip(), trace.SpanContext{})
			pool.enqueue(sampleTrip(), trace.SpanContext{})
			repo.EXPECT().IsStale(gomock.Any(), gomock.Any(), time.Minute).Return(true, nil)
			started := make(chan struct{})
			push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error { close(started); <-ctx.Done(); return ctx.Err() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, pool.Start(ctx))
			waitClosed(t, started)
			if deadline {
				expired, stop := context.WithDeadline(context.Background(), time.Time{})
				defer stop()
				require.ErrorIs(t, pool.Shutdown(expired), context.DeadlineExceeded)
			} else {
				cancel()
			}
			waitClosed(t, pool.done)
			waitClosed(t, ticks.stopped)
			require.Empty(t, pool.pending)
			require.Zero(t, pool.ActiveWorkers())
		})
	}
}

func TestTickerScansDoNotOverlap(t *testing.T) {
	pool, repo, _, ticks, _ := newTestPool(t, 1, 1)
	entered, release, second := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	repo.EXPECT().FindStaleBatch(gomock.Any(), time.Minute, gomock.Nil(), 100).DoAndReturn(func(ctx context.Context, _ time.Duration, _ *model.StaleTripCursor, _ int) ([]model.StaleTrip, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		} else {
			close(second)
		}
		return nil, nil
	}).Times(2)
	require.NoError(t, pool.Start(context.Background()))
	ticks.ticks <- time.Time{}
	waitClosed(t, entered)
	ticks.ticks <- time.Time{}
	require.Equal(t, int64(1), calls.Load())
	close(release)
	waitClosed(t, second)
}

func TestRecheckAndSpanLink(t *testing.T) {
	pool, repo, push, _, _ := newTestPool(t, 1, 2)
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) })
	trips := []model.StaleTrip{sampleTrip(), sampleTrip()}
	repo.EXPECT().FindStaleBatch(gomock.Any(), time.Minute, gomock.Nil(), 100).Return(trips, nil)
	repo.EXPECT().IsStale(gomock.Any(), trips[0].ID, time.Minute).Return(false, nil)
	repo.EXPECT().IsStale(gomock.Any(), trips[1].ID, time.Minute).Return(true, nil)
	push.EXPECT().RequestPosition(gomock.Any(), trips[1].DriverID, trips[1].ID).DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
		_, span := otel.Tracer("test").Start(ctx, "push")
		span.End()
		return nil
	})
	pool.scan(context.Background())
	pool.process(context.Background(), nextTask(t, pool))
	pool.process(context.Background(), nextTask(t, pool))
	spans := exporter.GetSpans()
	var scan trace.SpanContext
	for _, span := range spans {
		if span.Name == "positions.scan" {
			scan = span.SpanContext
		}
	}
	require.True(t, scan.IsValid())
	workers := 0
	for _, span := range spans {
		if span.Name != "positions.request" {
			continue
		}
		workers++
		require.False(t, span.Parent.IsValid(), "async task must start a new trace")
		require.NotEqual(t, scan.TraceID(), span.SpanContext.TraceID())
		require.Len(t, span.Links, 1)
		require.Equal(t, scan, span.Links[0].SpanContext)
	}
	require.Equal(t, 2, workers)
}

func TestConcurrentEnqueueCancelAndWork(t *testing.T) {
	pool, repo, push, _, _ := newTestPool(t, 4, 16)
	repo.EXPECT().IsStale(gomock.Any(), gomock.Any(), time.Minute).Return(true, nil).AnyTimes()
	started := make(chan struct{}, 1)
	push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, uuid.UUID, uuid.UUID) error {
			select {
			case started <- struct{}{}:
			default:
			}
			return nil
		},
	).AnyTimes()
	pool.enqueue(sampleTrip(), trace.SpanContext{})
	require.NoError(t, pool.Start(context.Background()))
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	trips := make([]model.StaleTrip, 16)
	for i := range trips {
		trips[i] = sampleTrip()
	}
	var requests sync.WaitGroup
	for caller := 0; caller < 8; caller++ {
		requests.Add(1)
		go func(offset int) {
			defer requests.Done()
			for i := 0; i < 200; i++ {
				trip := trips[(i+offset)%len(trips)]
				pool.enqueue(trip, trace.SpanContext{})
				if i%2 == 0 {
					pool.Cancel(trip.ID)
				}
			}
		}(caller)
	}
	requests.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, pool.Shutdown(ctx))
	require.Empty(t, pool.pending)
	require.Zero(t, pool.QueueSize())
	require.Zero(t, pool.ActiveWorkers())
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	return data
}

func sumValue(data metricdata.ResourceMetrics, name, result string) int64 {
	var count int64
	for _, scope := range data.ScopeMetrics {
		for _, value := range scope.Metrics {
			if value.Name != name {
				continue
			}
			if sum, ok := value.Data.(metricdata.Sum[int64]); ok {
				for _, point := range sum.DataPoints {
					if attr, ok := point.Attributes.Value(attribute.Key("result")); ok && attr.AsString() == result {
						count += point.Value
					}
				}
			}
		}
	}
	return count
}

func gaugeValue(data metricdata.ResourceMetrics, name string) int64 {
	for _, scope := range data.ScopeMetrics {
		for _, value := range scope.Metrics {
			if value.Name == name {
				if gauge, ok := value.Data.(metricdata.Gauge[int64]); ok && len(gauge.DataPoints) == 1 {
					return gauge.DataPoints[0].Value
				}
			}
		}
	}
	return -1
}
