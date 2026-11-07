package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/chugun223/TripGo/internal/database"
	"github.com/chugun223/TripGo/internal/domain"
)

type TripStatusHistoryRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTripStatusHistoryRepository(
	pool *pgxpool.Pool,
	queryTimeout time.Duration,
) *TripStatusHistoryRepository {
	return &TripStatusHistoryRepository{
		pool:         pool,
		queryTimeout: queryTimeout,
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

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	executor := database.ExecutorFromContext(queryCtx, r.pool)

	_, err = executor.Exec(queryCtx, query, args...)
	if err != nil {
		return fmt.Errorf("insert trip status history: %w", err)
	}

	return nil
}
