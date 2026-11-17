package position

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/postgres"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

type Repository struct {
	pool    *pgxpool.Pool
	metrics *telemetry.DBMetrics
}

func New(pool *pgxpool.Pool, metrics *telemetry.DBMetrics) *Repository {
	return &Repository{pool: pool, metrics: metrics}
}

func (r *Repository) Create(ctx context.Context, position trip.Position) (trip.Position, error) {
	query, args, err := sq.Insert("trip_positions").PlaceholderFormat(sq.Dollar).
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(position.TripID, position.Coordinates.Latitude, position.Coordinates.Longitude, position.RecordedAt).
		Suffix("RETURNING id").ToSql()
	if err != nil {
		return trip.Position{}, fmt.Errorf("build create position query: %w", err)
	}
	executor := postgres.ExecutorWithMetrics(ctx, r.pool, r.metrics)
	if err := executor.QueryRow(ctx, query, args...).Scan(&position.ID); err != nil {
		return trip.Position{}, fmt.Errorf("create position: %w", err)
	}
	return position, nil
}

func (r *Repository) List(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error) {
	query, args, err := sq.Select("id", "trip_id", "latitude", "longitude", "recorded_at").
		PlaceholderFormat(sq.Dollar).From("trip_positions").Where(sq.Eq{"trip_id": tripID}).
		OrderBy("recorded_at", "id").ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list positions query: %w", err)
	}
	executor := postgres.ExecutorWithMetrics(ctx, r.pool, r.metrics)
	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	defer rows.Close()
	positions := make([]trip.Position, 0)
	for rows.Next() {
		var position trip.Position
		if err := rows.Scan(&position.ID, &position.TripID, &position.Coordinates.Latitude,
			&position.Coordinates.Longitude, &position.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read positions: %w", err)
	}
	return positions, nil
}
