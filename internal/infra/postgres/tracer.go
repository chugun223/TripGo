package postgres

import (
	"context"
	"errors"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type collectionNameTracer struct {
	*otelpgx.Tracer
}

type queryOperationContextKey struct{}

func newCollectionNameTracer() *collectionNameTracer {
	return &collectionNameTracer{
		Tracer: otelpgx.NewTracer(
			otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		),
	}
}

func (t *collectionNameTracer) TraceQueryStart(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)

	operation, collection := classifySQL(data.SQL)

	ctx = context.WithValue(ctx, queryOperationContextKey{}, operation)

	if collection != "" && collection != unknownCollection {
		span := trace.SpanFromContext(ctx)

		span.SetAttributes(
			attribute.String("db.collection.name", collection),
		)
	}

	return ctx
}

func (t *collectionNameTracer) TraceQueryEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceQueryEndData,
) {
	span := trace.SpanFromContext(ctx)

	var errorType string

	if data.Err != nil {
		errorType = classifyErrorType(data.Err)
	} else {
		operation, _ := ctx.Value(queryOperationContextKey{}).(string)

		if operation == "SELECT" && data.CommandTag.RowsAffected() == 0 {
			errorType = errorTypeNoRows
		}
	}

	if errorType != "" {
		span.SetAttributes(
			attribute.String("error.type", errorType),
		)
	}

	t.Tracer.TraceQueryEnd(ctx, conn, data)
}

func classifyErrorType(err error) string {
	var pgErr *pgconn.PgError

	switch {
	case errors.As(err, &pgErr):
		return pgErr.Code

	case errors.Is(err, pgx.ErrNoRows):
		return errorTypeNoRows

	default:
		return "other"
	}
}
