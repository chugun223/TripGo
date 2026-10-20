package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
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
	executor, ok := ctx.Value(executorKey{}).(Executor)
	if ok {
		return executor
	}

	return pool
}

func executorFromContext(ctx context.Context) (Executor, bool) {
	executor, ok := ctx.Value(executorKey{}).(Executor)
	return executor, ok
}
