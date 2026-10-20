package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/chugun223/TripGo/internal/database"
	"github.com/chugun223/TripGo/internal/domain"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type TripRepository struct {
	pool *pgxpool.Pool
}

func NewTripRepository(pool *pgxpool.Pool) *TripRepository {
	return &TripRepository{pool: pool}
}

func (r *TripRepository) Create(
	ctx context.Context,
	trip domain.Trip,
) (domain.Trip, error) {
	trip.Status = domain.TripStatusActive
	trip.StartedAt = time.Now().UTC()
	trip.FinishedAt = nil

	query, args, err := psql.
		Insert("trips").
		Columns(
			"id",
			"user_id",
			"driver_id",
			"start_latitude",
			"start_longitude",
			"end_latitude",
			"end_longitude",
			"price",
			"status",
			"started_at",
			"finished_at",
		).
		Values(
			trip.ID,
			trip.UserID,
			trip.DriverID,
			trip.StartPoint.Latitude,
			trip.StartPoint.Longitude,
			trip.EndPoint.Latitude,
			trip.EndPoint.Longitude,
			trip.Price,
			trip.Status,
			trip.StartedAt,
			nil,
		).
		Suffix(`
			RETURNING
				id, user_id, driver_id,
				start_latitude, start_longitude,
				end_latitude, end_longitude,
				price, status, started_at, finished_at,
				created_at, updated_at
		`).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build create trip query: %w", err)
	}

	executor := database.ExecutorFromContext(ctx, r.pool)

	createdTrip, err := scanTrip(executor.QueryRow(ctx, query, args...))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "trips_active_driver_uidx" {
			return domain.Trip{}, domain.ErrDriverBusy
		}
		return domain.Trip{}, fmt.Errorf("insert trip: %w", err)
	}

	return createdTrip, nil
}

func (r *TripRepository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (domain.Trip, error) {
	query, args, err := psql.
		Select(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude",
			"end_latitude", "end_longitude",
			"price", "status", "started_at", "finished_at",
			"created_at", "updated_at",
		).
		From("trips").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	executor := database.ExecutorFromContext(ctx, r.pool)

	trip, err := scanTrip(executor.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, domain.ErrTripNotFound
	}
	if err != nil {
		return domain.Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return trip, nil
}

func (r *TripRepository) Finish(
	ctx context.Context,
	id uuid.UUID,
) (domain.Trip, error) {
	now := time.Now().UTC()

	query, args, err := psql.
		Update("trips").
		Set("status", domain.TripStatusCompleted).
		Set("finished_at", now).
		Set("updated_at", now).
		Where(sq.Eq{
			"id":     id,
			"status": domain.TripStatusActive,
		}).
		Suffix(`
			RETURNING
				id, user_id, driver_id,
				start_latitude, start_longitude,
				end_latitude, end_longitude,
				price, status, started_at, finished_at,
				created_at, updated_at
		`).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}

	executor := database.ExecutorFromContext(ctx, r.pool)

	trip, err := scanTrip(executor.QueryRow(ctx, query, args...))
	if err == nil {
		return trip, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	existingTrip, getErr := r.GetByID(ctx, id)
	if errors.Is(getErr, domain.ErrTripNotFound) {
		return domain.Trip{}, domain.ErrTripNotFound
	}
	if getErr != nil {
		return domain.Trip{}, fmt.Errorf("check trip after finish: %w", getErr)
	}

	if existingTrip.Status == domain.TripStatusCompleted {
		return domain.Trip{}, domain.ErrTripCompleted
	}

	return domain.Trip{}, errors.New(
		"trip was not finished although it is still active",
	)
}

func scanTrip(row pgx.Row) (domain.Trip, error) {
	var trip domain.Trip

	err := row.Scan(
		&trip.ID,
		&trip.UserID,
		&trip.DriverID,
		&trip.StartPoint.Latitude,
		&trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude,
		&trip.EndPoint.Longitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
		&trip.CreatedAt,
		&trip.UpdatedAt,
	)
	if err != nil {
		return domain.Trip{}, err
	}

	return trip, nil
}
