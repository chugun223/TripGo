package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/postgres"
	"github.com/chugun223/TripGo/internal/model"
)

func (r *TripRepository) LockActive(ctx context.Context, id uuid.UUID) error {
	query, args, err := psql.Select("status").From("trips").
		Where(sq.Eq{"id": id}).Suffix("FOR UPDATE").ToSql()
	if err != nil {
		return fmt.Errorf("build lock active trip query: %w", err)
	}
	var status domain.TripStatus
	executor := postgres.ExecutorWithMetrics(ctx, r.pool, r.metrics)
	err = executor.QueryRow(ctx, query, args...).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrTripNotFound
	}
	if err != nil {
		return fmt.Errorf("lock active trip: %w", err)
	}
	if status != domain.TripStatusActive {
		return domain.ErrTripCompleted
	}
	return nil
}

func (r *TripRepository) UpdateLastPosition(ctx context.Context, id uuid.UUID, recordedAt time.Time) error {
	query, args, err := psql.Update("trips").
		Set("last_position_at", sq.Expr("GREATEST(last_position_at, ?)", recordedAt)).
		Set("updated_at", sq.Expr("now()")).Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return fmt.Errorf("build update last position query: %w", err)
	}
	executor := postgres.ExecutorWithMetrics(ctx, r.pool, r.metrics)
	if _, err := executor.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("update last position: %w", err)
	}
	return nil
}

func (r *TripRepository) FindStaleBatch(ctx context.Context, staleAfter time.Duration, after *model.StaleTripCursor, limit int) ([]model.StaleTrip, error) {
	if limit <= 0 {
		return nil, errors.New("stale trip batch size must be positive")
	}
	query, args, err := staleBatchQuery(staleAfter, after, uint64(limit)).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build stale trips query: %w", err)
	}
	executor := postgres.ExecutorWithMetrics(ctx, r.pool, r.metrics)
	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find stale trips: %w", err)
	}
	defer rows.Close()
	trips := make([]model.StaleTrip, 0)
	for rows.Next() {
		var trip model.StaleTrip
		if err := rows.Scan(&trip.ID, &trip.DriverID, &trip.LastActivityAt); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}
		trips = append(trips, trip)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read stale trips: %w", err)
	}
	return trips, nil
}

func staleBatchQuery(staleAfter time.Duration, after *model.StaleTripCursor, limit uint64) sq.SelectBuilder {
	query := staleQuery(staleAfter).
		Columns("id", "driver_id", "COALESCE(last_position_at, started_at)").
		OrderBy("COALESCE(last_position_at, started_at)", "id").Limit(limit)
	if after != nil {
		query = query.Where(sq.Expr("(COALESCE(last_position_at, started_at), id) > (?, ?)", after.LastActivityAt, after.ID))
	}
	return query
}

func (r *TripRepository) IsStale(ctx context.Context, id uuid.UUID, staleAfter time.Duration) (bool, error) {
	query, args, err := staleQuery(staleAfter).Columns("id").Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return false, fmt.Errorf("build check stale trip query: %w", err)
	}
	executor := postgres.ExecutorWithMetrics(ctx, r.pool, r.metrics)
	var found uuid.UUID
	if err := executor.QueryRow(ctx, query, args...).Scan(&found); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check stale trip: %w", err)
	}
	return true, nil
}

func staleQuery(staleAfter time.Duration) sq.SelectBuilder {
	return psql.Select().From("trips").
		Where("status = 'active'").
		Where(sq.Expr(
			"COALESCE(last_position_at, started_at) < now() - (?::bigint * INTERVAL '1 microsecond')",
			staleAfter.Microseconds(),
		))
}
