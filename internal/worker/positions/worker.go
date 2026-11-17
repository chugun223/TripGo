package positions

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/chugun223/TripGo/internal/infra/logger"
)

func (p *Pool) work(ctx context.Context) {
	for {
		queued, ok := p.take(ctx)
		if !ok {
			return
		}
		p.process(ctx, queued)
	}
}

func (p *Pool) process(parent context.Context, queued *task) {
	ctx, cancel := context.WithTimeout(parent, p.settings.RequestTimeout)
	defer cancel()
	p.mu.Lock()
	if p.pending[queued.trip.ID] != queued {
		p.mu.Unlock()
		return
	}
	queued.cancel = cancel
	p.mu.Unlock()
	defer p.finish(queued)
	p.active.Add(1)
	defer p.active.Add(-1)
	options := []trace.SpanStartOption{trace.WithNewRoot()}
	if queued.link.IsValid() {
		options = append(options, trace.WithLinks(trace.Link{SpanContext: queued.link}))
	}
	ctx, span := otel.Tracer("tripgo/positions").Start(ctx, "positions.request", options...)
	defer span.End()
	queryCtx, queryCancel := context.WithTimeout(ctx, p.settings.QueryTimeout)
	stale, err := p.repository.IsStale(queryCtx, queued.trip.ID, p.settings.StaleAfter)
	queryCancel()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check stale trip")
		span.SetAttributes(attribute.String("error.type", "database"))
		logger.WithTrace(ctx, p.log).ErrorContext(ctx, "failed to check queued trip", slog.Any("error", err))
		return
	}
	if !stale || ctx.Err() != nil {
		return
	}
	start := time.Now()
	err = p.push.RequestPosition(ctx, queued.trip.DriverID, queued.trip.ID)
	result := "success"
	if err != nil {
		result = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, "request position")
		span.SetAttributes(attribute.String("error.type", "push"))
		logger.WithTrace(ctx, p.log).WarnContext(ctx, "position request failed",
			slog.String("trip_id", queued.trip.ID.String()), slog.Any("error", err))
	}
	metricOptions := metric.WithAttributes(attribute.String("result", result))
	p.metrics.Requests.Add(ctx, 1, metricOptions)
	p.metrics.RequestDuration.Record(ctx, time.Since(start).Seconds(), metricOptions)
}

func (p *Pool) finish(queued *task) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending[queued.trip.ID] == queued {
		delete(p.pending, queued.trip.ID)
	}
}
