//go:build integration

package trip

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
	"github.com/chugun223/TripGo/internal/infra/postgres"
	"github.com/chugun223/TripGo/internal/infra/postgres/transaction"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
	"github.com/chugun223/TripGo/internal/model"
	positionrepo "github.com/chugun223/TripGo/internal/repository/position"
	finishusecase "github.com/chugun223/TripGo/internal/usecase/trip/finish"
	createposition "github.com/chugun223/TripGo/internal/usecase/trip/position/create"
	listpositions "github.com/chugun223/TripGo/internal/usecase/trip/position/list"
)

type positionTestDB struct {
	ctx       context.Context
	pool      *pgxpool.Pool
	trips     *TripRepository
	positions *positionrepo.Repository
	tx        transaction.TxManager
	queue     *testPositionQueue
}

type testPositionQueue struct{ canceled []uuid.UUID }

func (q *testPositionQueue) Cancel(id uuid.UUID) { q.canceled = append(q.canceled, id) }

func newPositionTestDB(t *testing.T) positionTestDB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	schema := pgx.Identifier{"lab3_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
		admin.Close()
	})
	files, err := filepath.Glob("../../../migrations/*.sql")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		up := strings.SplitN(string(content), "-- +goose Down", 2)[0]
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err, "migration %s", file)
	}
	metrics, err := telemetry.NewDBMetrics()
	require.NoError(t, err)
	return positionTestDB{
		ctx: ctx, pool: pool, trips: NewTripRepository(pool, metrics), positions: positionrepo.New(pool, metrics),
		tx: transaction.NewTxManager(pool), queue: &testPositionQueue{},
	}
}

func (db positionTestDB) trip(t *testing.T) domain.Trip {
	t.Helper()
	value, err := db.trips.Create(db.ctx, domain.Trip{ID: uuid.New(), UserID: uuid.New(), DriverID: uuid.New(), Price: 500})
	require.NoError(t, err)
	return value
}

func (db positionTestDB) creator() *createposition.Usecase {
	return createposition.New(db.trips, db.positions, db.tx, db.queue, time.Now, 5*time.Second)
}

func TestPositionsOrderAndMonotonicLastPosition(t *testing.T) {
	db := newPositionTestDB(t)
	value := db.trip(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	uc := db.creator()
	saved := make([]domain.Position, 0, 3)
	for _, at := range []time.Time{now.Add(-10 * time.Second), now.Add(-time.Minute), now.Add(-10 * time.Second)} {
		position, err := uc.Execute(db.ctx, model.CreatePositionInput{TripID: value.ID, RecordedAt: at})
		require.NoError(t, err)
		saved = append(saved, position)
	}
	fetched, err := db.trips.GetByID(db.ctx, value.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched.LastPositionAt)
	require.True(t, saved[0].RecordedAt.Equal(*fetched.LastPositionAt))
	positions, err := listpositions.New(db.trips, db.positions, time.Second).Execute(db.ctx, value.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{saved[1].ID, saved[0].ID, saved[2].ID}, []int64{positions[0].ID, positions[1].ID, positions[2].ID})
	require.Len(t, db.queue.canceled, 3)
	_, err = db.trips.Finish(db.ctx, value.ID)
	require.NoError(t, err)
	positions, err = listpositions.New(db.trips, db.positions, time.Second).Execute(db.ctx, value.ID)
	require.NoError(t, err)
	require.Len(t, positions, 3, "completed trip route is still readable")
}

type failingPositionUpdate struct {
	*TripRepository
	pool *pgxpool.Pool
}

func (r failingPositionUpdate) UpdateLastPosition(ctx context.Context, id uuid.UUID, _ time.Time) error {
	_, err := postgres.ExecutorFromContext(ctx, r.pool).Exec(ctx, "UPDATE trips SET price = -1 WHERE id = $1", id)
	return err
}

func TestPositionInsertRollsBackWhenTripUpdateFails(t *testing.T) {
	db := newPositionTestDB(t)
	value := db.trip(t)
	uc := createposition.New(failingPositionUpdate{TripRepository: db.trips, pool: db.pool}, db.positions, db.tx,
		db.queue, time.Now, time.Second)
	_, err := uc.Execute(db.ctx, model.CreatePositionInput{TripID: value.ID, RecordedAt: time.Now().UTC()})
	require.Error(t, err)
	positions, err := db.positions.List(db.ctx, value.ID)
	require.NoError(t, err)
	require.Empty(t, positions, "the INSERT before the failed UPDATE must be rolled back")
	fetched, err := db.trips.GetByID(db.ctx, value.ID)
	require.NoError(t, err)
	require.Nil(t, fetched.LastPositionAt)
	require.Equal(t, int64(500), fetched.Price)
	require.Empty(t, db.queue.canceled, "a rolled back coordinate must not cancel a queued push")
}

func TestPositionsRejectMissingAndCompletedTrips(t *testing.T) {
	db := newPositionTestDB(t)
	uc := db.creator()
	_, err := uc.Execute(db.ctx, model.CreatePositionInput{TripID: uuid.New(), RecordedAt: time.Now()})
	require.ErrorIs(t, err, domain.ErrTripNotFound)
	value := db.trip(t)
	_, err = db.trips.Finish(db.ctx, value.ID)
	require.NoError(t, err)
	_, err = uc.Execute(db.ctx, model.CreatePositionInput{TripID: value.ID, RecordedAt: time.Now()})
	require.ErrorIs(t, err, domain.ErrTripCompleted)
	positions, err := db.positions.List(db.ctx, value.ID)
	require.NoError(t, err)
	require.Empty(t, positions)
}

func TestStaleTripSelection(t *testing.T) {
	db := newPositionTestDB(t)
	now := time.Now().UTC()
	oldNoPosition, oldPosition := db.trip(t), db.trip(t)
	freshNoPosition, freshPosition, completed := db.trip(t), db.trip(t), db.trip(t)
	for _, fixture := range []struct {
		id      uuid.UUID
		started time.Time
		last    *time.Time
	}{
		{oldNoPosition.ID, now.Add(-4 * time.Minute), nil},
		{oldPosition.ID, now.Add(-10 * time.Minute), timePointer(now.Add(-3 * time.Minute))},
		{freshNoPosition.ID, now.Add(-5 * time.Second), nil},
		{freshPosition.ID, now.Add(-10 * time.Minute), timePointer(now.Add(-5 * time.Second))},
		{completed.ID, now.Add(-10 * time.Minute), nil},
	} {
		_, err := db.pool.Exec(db.ctx, "UPDATE trips SET started_at=$2,last_position_at=$3 WHERE id=$1", fixture.id, fixture.started, fixture.last)
		require.NoError(t, err)
	}
	_, err := db.trips.Finish(db.ctx, completed.ID)
	require.NoError(t, err)
	stale, err := db.trips.FindStaleBatch(db.ctx, time.Minute, nil, 100)
	require.NoError(t, err)
	require.Len(t, stale, 2)
	require.Equal(t, []uuid.UUID{oldNoPosition.ID, oldPosition.ID}, []uuid.UUID{stale[0].ID, stale[1].ID})
	for _, id := range []uuid.UUID{oldNoPosition.ID, oldPosition.ID} {
		found, err := db.trips.IsStale(db.ctx, id, time.Minute)
		require.NoError(t, err)
		require.True(t, found)
	}
	for _, id := range []uuid.UUID{freshNoPosition.ID, freshPosition.ID, completed.ID, uuid.New()} {
		found, err := db.trips.IsStale(db.ctx, id, time.Minute)
		require.NoError(t, err)
		require.False(t, found)
	}
}

func TestStaleTripKeysetBatchesWithEqualActivity(t *testing.T) {
	db := newPositionTestDB(t)
	at := time.Now().UTC().Truncate(time.Microsecond).Add(-10 * time.Minute)
	_, err := db.pool.Exec(db.ctx, `INSERT INTO trips
		(id,user_id,driver_id,start_latitude,start_longitude,end_latitude,end_longitude,price,status,started_at)
		SELECT md5(i::text)::uuid,md5('user'||i)::uuid,md5('driver'||i)::uuid,0,0,0,0,100,'active',$1
		FROM generate_series(1,250) AS i`, at)
	require.NoError(t, err)
	var after *model.StaleTripCursor
	seen := make(map[uuid.UUID]bool, 250)
	var previous string
	for _, size := range []int{100, 100, 50} {
		batch, err := db.trips.FindStaleBatch(db.ctx, time.Minute, after, 100)
		require.NoError(t, err)
		require.Len(t, batch, size)
		for _, trip := range batch {
			require.True(t, trip.LastActivityAt.Equal(at))
			require.NotEqual(t, uuid.Nil, trip.DriverID)
			require.False(t, seen[trip.ID], "duplicate across keyset pages")
			require.Greater(t, trip.ID.String(), previous)
			seen[trip.ID] = true
			previous = trip.ID.String()
		}
		last := batch[len(batch)-1]
		after = &model.StaleTripCursor{LastActivityAt: last.LastActivityAt, ID: last.ID}
	}
	require.Len(t, seen, 250)
	empty, err := db.trips.FindStaleBatch(db.ctx, time.Minute, after, 100)
	require.NoError(t, err)
	require.Empty(t, empty)
	_, err = db.trips.FindStaleBatch(db.ctx, time.Minute, nil, 0)
	require.Error(t, err)
}

func timePointer(at time.Time) *time.Time { return &at }

func TestStaleTripQueryUsesIndex(t *testing.T) {
	db := newPositionTestDB(t)
	_, err := db.pool.Exec(db.ctx, `INSERT INTO trips
		(id,user_id,driver_id,start_latitude,start_longitude,end_latitude,end_longitude,price,status,started_at)
		SELECT md5(i::text)::uuid,md5('user'||i)::uuid,md5('driver'||i)::uuid,0,0,0,0,100,'active',
		CASE WHEN i<=5 THEN now()-INTERVAL '2 minutes' ELSE now() END
		FROM generate_series(1,20000) AS i`)
	require.NoError(t, err)
	_, err = db.pool.Exec(db.ctx, "ANALYZE trips")
	require.NoError(t, err)
	query, args, err := staleBatchQuery(time.Minute, nil, 100).ToSql()
	require.NoError(t, err)
	rows, err := db.pool.Query(db.ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query, args...)
	require.NoError(t, err)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}
	require.NoError(t, rows.Err())
	require.Contains(t, plan.String(), "trips_stale_positions_idx")
	require.NotContains(t, plan.String(), "Seq Scan")
	t.Log(plan.String())
}

func TestPositionConcurrentWithFinish(t *testing.T) {
	db := newPositionTestDB(t)
	metrics, err := telemetry.NewDBMetrics()
	require.NoError(t, err)
	finish := finishusecase.New(db.trips, NewTripStatusHistoryRepository(db.pool, metrics), db.tx)
	for i := 0; i < 10; i++ {
		value := db.trip(t)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var positionErr, finishErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, positionErr = db.creator().Execute(db.ctx, model.CreatePositionInput{TripID: value.ID, RecordedAt: time.Now()})
		}()
		go func() { defer wg.Done(); <-start; _, finishErr = finish.Execute(db.ctx, value.ID) }()
		close(start)
		wg.Wait()
		require.NoError(t, finishErr)
		require.True(t, positionErr == nil || errors.Is(positionErr, domain.ErrTripCompleted), "%v", positionErr)
		fetched, err := db.trips.GetByID(db.ctx, value.ID)
		require.NoError(t, err)
		require.Equal(t, domain.TripStatusCompleted, fetched.Status)
		positions, err := db.positions.List(db.ctx, value.ID)
		require.NoError(t, err)
		if positionErr == nil {
			require.Len(t, positions, 1)
		} else {
			require.Empty(t, positions)
		}
	}
}
