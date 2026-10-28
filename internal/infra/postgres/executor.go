package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

type Executor interface {
	Exec(
		ctx context.Context,
		sql string,
		arguments ...any,
	) (pgconn.CommandTag, error)

	Query(
		ctx context.Context,
		sql string,
		arguments ...any,
	) (pgx.Rows, error)

	QueryRow(
		ctx context.Context,
		sql string,
		arguments ...any,
	) pgx.Row
}

type executorKey struct{}

func ContextWithExecutor(
	ctx context.Context,
	executor Executor,
) context.Context {
	return context.WithValue(ctx, executorKey{}, executor)
}

func ExecutorFromContext(
	ctx context.Context,
	pool *pgxpool.Pool,
) Executor {
	if executor, ok := ExecutorFromContextIfPresent(ctx); ok {
		return executor
	}

	return pool
}

func ExecutorFromContextIfPresent(
	ctx context.Context,
) (Executor, bool) {
	executor, ok := ctx.Value(executorKey{}).(Executor)
	return executor, ok
}

func ExecutorWithMetrics(
	ctx context.Context,
	pool *pgxpool.Pool,
	metrics *telemetry.DBMetrics,
) Executor {
	executor := ExecutorFromContext(ctx, pool)

	return NewMetricsExecutor(executor, metrics)
}
