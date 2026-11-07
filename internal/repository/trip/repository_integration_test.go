//go:build integration

package trip

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/postgres/transaction"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

func TestCreateTripRollbackWhenHistoryFails(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	dbMetrics, err := telemetry.NewDBMetrics()
	if err != nil {
		t.Fatalf("initialize database metrics: %v", err)
	}

	tripRepository := NewTripRepository(pool, dbMetrics)
	historyRepository := NewTripStatusHistoryRepository(pool, dbMetrics)
	txManager := transaction.NewTxManager(pool)

	tripID := uuid.New()

	newTrip := domain.Trip{
		ID:       tripID,
		UserID:   uuid.New(),
		DriverID: uuid.New(),
		StartPoint: domain.Coordinates{
			Latitude:  56.8389,
			Longitude: 60.6057,
		},
		EndPoint: domain.Coordinates{
			Latitude:  56.8500,
			Longitude: 60.6100,
		},
		Price: 1000,
	}

	invalidStatus := domain.TripStatus("invalid")

	err = txManager.Do(ctx, func(txCtx context.Context) error {
		created, createErr := tripRepository.Create(txCtx, newTrip)
		if createErr != nil {
			return createErr
		}

		return historyRepository.Create(
			txCtx,
			created.ID,
			nil,
			invalidStatus,
			nil,
		)
	})

	if err == nil {
		t.Fatal("expected transaction error")
	}

	_, err = tripRepository.GetByID(ctx, tripID)

	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf(
			"expected trip to be rolled back, got %v",
			err,
		)
	}
}

func TestCountActiveTrips(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	dbMetrics, err := telemetry.NewDBMetrics()
	if err != nil {
		t.Fatalf("initialize database metrics: %v", err)
	}

	repository := NewTripRepository(pool, dbMetrics)

	before, err := repository.CountActive(ctx)
	if err != nil {
		t.Fatalf("count active trips before insert: %v", err)
	}

	tripID := uuid.New()

	newTrip := domain.Trip{
		ID:       tripID,
		UserID:   uuid.New(),
		DriverID: uuid.New(),
		StartPoint: domain.Coordinates{
			Latitude:  56.8389,
			Longitude: 60.6057,
		},
		EndPoint: domain.Coordinates{
			Latitude:  56.8500,
			Longitude: 60.6100,
		},
		Price: 1000,
	}

	created, err := repository.Create(ctx, newTrip)
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		if _, err := pool.Exec(
			cleanupCtx,
			"DELETE FROM trips WHERE id = $1",
			tripID,
		); err != nil {
			t.Errorf("cleanup trip: %v", err)
		}
	})

	afterCreate, err := repository.CountActive(ctx)
	if err != nil {
		t.Fatalf("count active trips after create: %v", err)
	}

	if afterCreate != before+1 {
		t.Fatalf(
			"active trips after create = %d, want %d",
			afterCreate,
			before+1,
		)
	}

	_, err = repository.Finish(ctx, created.ID)
	if err != nil {
		t.Fatalf("finish trip: %v", err)
	}

	afterFinish, err := repository.CountActive(ctx)
	if err != nil {
		t.Fatalf("count active trips after finish: %v", err)
	}

	if afterFinish != before {
		t.Fatalf(
			"active trips after finish = %d, want %d",
			afterFinish,
			before,
		)
	}
}
