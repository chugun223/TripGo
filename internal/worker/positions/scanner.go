package positions

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/chugun223/TripGo/internal/infra/logger"
	"github.com/chugun223/TripGo/internal/model"
)

const (
	resultQueued    = "queued"
	resultDuplicate = "duplicate"
	resultDropped   = "dropped"
)

func (p *Pool) scanLoop(ctx context.Context, ticks ticker, done chan<- struct{}) {
	defer close(done)
	defer p.closeQueue()
	defer ticks.Stop()
	p.log.InfoContext(ctx, "position scanner started")
	defer p.log.InfoContext(ctx, "position scanner stopped")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks.Ticks():
			if ctx.Err() == nil {
				p.scan(ctx)
			}
		}
	}
}

func (p *Pool) scan(ctx context.Context) {
	ctx, span := otel.Tracer("tripgo/positions").Start(ctx, "positions.scan")
	defer span.End()
	var rowsRead int64
	defer func() { p.metrics.ScanRows.Record(ctx, rowsRead) }()
	var after *model.StaleTripCursor
	for ctx.Err() == nil {
		queryCtx, cancel := context.WithTimeout(ctx, p.settings.QueryTimeout)
		trips, err := p.repository.FindStaleBatch(queryCtx, p.settings.StaleAfter, after, p.settings.BatchSize)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "find stale trips")
				span.SetAttributes(attribute.String("error.type", "database"))
				logger.WithTrace(ctx, p.log).ErrorContext(ctx, "failed to find stale trips", slog.Any("error", err))
			}
			return
		}
		rowsRead += int64(len(trips))
		for _, trip := range trips {
			if ctx.Err() != nil {
				return
			}
			result, evicted := p.enqueue(trip, trace.SpanContextFromContext(ctx))
			p.metrics.StaleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
			if result == resultDropped {
				p.recordDropped(ctx, trip.ID)
			}
			if evicted != nil {
				p.recordDropped(ctx, evicted.trip.ID)
			}
		}
		if len(trips) < p.settings.BatchSize || p.QueueSize() >= int64(p.settings.QueueSize) {
			return
		}
		last := trips[len(trips)-1]
		after = &model.StaleTripCursor{LastActivityAt: last.LastActivityAt, ID: last.ID}
	}
}

func (p *Pool) recordDropped(ctx context.Context, id uuid.UUID) {
	p.metrics.Requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", resultDropped)))
	logger.WithTrace(ctx, p.log).WarnContext(ctx, "position request dropped from full queue", slog.String("trip_id", id.String()))
}
