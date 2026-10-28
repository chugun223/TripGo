package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

type MetricsExecutor struct {
	next    Executor
	metrics *telemetry.DBMetrics
}

func NewMetricsExecutor(
	next Executor,
	metrics *telemetry.DBMetrics,
) *MetricsExecutor {
	return &MetricsExecutor{
		next:    next,
		metrics: metrics,
	}
}

func (e *MetricsExecutor) Exec(
	ctx context.Context,
	sql string,
	args ...any,
) (pgconn.CommandTag, error) {
	start := time.Now()

	tag, err := e.next.Exec(ctx, sql, args...)

	e.record(ctx, sql, time.Since(start), err)

	return tag, err
}

func (e *MetricsExecutor) QueryRow(
	ctx context.Context,
	sql string,
	args ...any,
) pgx.Row {
	start := time.Now()

	row := e.next.QueryRow(ctx, sql, args...)

	return &metricsRow{
		row:      row,
		ctx:      ctx,
		sql:      sql,
		start:    start,
		executor: e,
	}
}

func (e *MetricsExecutor) Query(
	ctx context.Context,
	sql string,
	args ...any,
) (pgx.Rows, error) {
	start := time.Now()

	rows, err := e.next.Query(ctx, sql, args...)
	if err != nil {
		e.record(ctx, sql, time.Since(start), err)
		return nil, err
	}

	return &metricsRows{
		Rows:     rows,
		ctx:      ctx,
		sql:      sql,
		start:    start,
		executor: e,
	}, nil
}

type metricsRow struct {
	row      pgx.Row
	ctx      context.Context
	sql      string
	start    time.Time
	executor *MetricsExecutor
	recorded bool
}

type metricsRows struct {
	pgx.Rows

	ctx      context.Context
	sql      string
	start    time.Time
	executor *MetricsExecutor

	recorded bool
}

func (r *metricsRows) Close() {
	r.Rows.Close()
	r.record()
}

func (r *metricsRows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		r.record()
	}

	return ok
}

func (r *metricsRows) record() {
	if r.recorded {
		return
	}

	r.recorded = true

	r.executor.record(
		r.ctx,
		r.sql,
		time.Since(r.start),
		r.Err(),
	)
}

func (r *metricsRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)

	if !r.recorded {
		r.recorded = true

		r.executor.record(
			r.ctx,
			r.sql,
			time.Since(r.start),
			err,
		)
	}

	return err
}

func (e *MetricsExecutor) record(
	ctx context.Context,
	sql string,
	duration time.Duration,
	err error,
) {
	operation, collection := classifySQL(sql)

	attrs := []attribute.KeyValue{
		attribute.String("db.operation.name", operation),
		attribute.String("db.collection.name", collection),
	}

	if err != nil {
		var pgErr *pgconn.PgError

		switch {
		case errors.As(err, &pgErr):
			attrs = append(
				attrs,
				attribute.String("error.type", pgErr.Code),
			)

		case errors.Is(err, pgx.ErrNoRows):
			attrs = append(
				attrs,
				attribute.String("error.type", "no_rows"),
			)

		default:
			attrs = append(
				attrs,
				attribute.String("error.type", "other"),
			)
		}
	}

	e.metrics.OperationDuration.Record(
		ctx,
		duration.Seconds(),
		metric.WithAttributes(attrs...),
	)
}
