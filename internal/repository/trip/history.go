package trip

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/postgres"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

type TripStatusHistoryRepository struct {
	pool    *pgxpool.Pool
	metrics *telemetry.DBMetrics
}

func NewTripStatusHistoryRepository(
	pool *pgxpool.Pool,
	metrics *telemetry.DBMetrics,
) *TripStatusHistoryRepository {
	return &TripStatusHistoryRepository{
		pool:    pool,
		metrics: metrics,
	}
}

func (r *TripStatusHistoryRepository) Create(
	ctx context.Context,
	tripID uuid.UUID,
	fromStatus *domain.TripStatus,
	toStatus domain.TripStatus,
	reason *string,
) error {
	query, args, err := psql.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, fromStatus, toStatus, reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create status history query: %w", err)
	}

	executor := postgres.ExecutorWithMetrics(ctx, r.pool, r.metrics)

	_, err = executor.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("insert trip status history: %w", err)
	}

	return nil
}
