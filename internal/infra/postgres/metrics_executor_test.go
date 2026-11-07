package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	metricdata "go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

type fakeExecutor struct {
	execErr  error
	queryErr error
	row      pgx.Row
	rows     pgx.Rows
}

func (f *fakeExecutor) Exec(
	_ context.Context,
	_ string,
	_ ...any,
) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, f.execErr
}

func (f *fakeExecutor) Query(
	_ context.Context,
	_ string,
	_ ...any,
) (pgx.Rows, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}

	return f.rows, nil
}

func (f *fakeExecutor) QueryRow(
	_ context.Context,
	_ string,
	_ ...any,
) pgx.Row {
	return f.row
}

type fakeRow struct {
	err error
}

func (r *fakeRow) Scan(_ ...any) error {
	return r.err
}

func newTestMetricsExecutor(
	t *testing.T,
	next Executor,
) (*MetricsExecutor, *metric.ManualReader) {
	t.Helper()

	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(
		metric.WithReader(reader),
	)

	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)

	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	dbMetrics, err := telemetry.NewDBMetrics()
	if err != nil {
		t.Fatalf("create database metrics: %v", err)
	}

	return NewMetricsExecutor(next, dbMetrics), reader
}

func getDBHistogram(
	t *testing.T,
	reader *metric.ManualReader,
) metricdata.Histogram[float64] {
	t.Helper()

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "db.client.operation.duration" {
				continue
			}

			if m.Unit != "s" {
				t.Fatalf("unexpected metric unit: %s", m.Unit)
			}

			histogram, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("unexpected metric data type: %T", m.Data)
			}

			return histogram
		}
	}

	t.Fatal("database operation duration metric not found")
	return metricdata.Histogram[float64]{}
}

func requireAttribute(
	t *testing.T,
	attrs attribute.Set,
	key string,
	expected string,
) {
	t.Helper()

	value, ok := attrs.Value(attribute.Key(key))
	if !ok {
		t.Fatalf("attribute %s not found", key)
	}

	if value.AsString() != expected {
		t.Fatalf(
			"attribute %s: got %q, want %q",
			key,
			value.AsString(),
			expected,
		)
	}
}

func TestMetricsExecutorExec(t *testing.T) {
	executor, reader := newTestMetricsExecutor(
		t,
		&fakeExecutor{},
	)

	_, err := executor.Exec(
		context.Background(),
		"INSERT INTO trip_status_history (trip_id) VALUES ($1)",
		"trip-id",
	)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}

	histogram := getDBHistogram(t, reader)

	if len(histogram.DataPoints) != 1 {
		t.Fatalf(
			"expected 1 data point, got %d",
			len(histogram.DataPoints),
		)
	}

	point := histogram.DataPoints[0]

	if point.Count != 1 {
		t.Fatalf("expected count 1, got %d", point.Count)
	}

	requireAttribute(
		t,
		point.Attributes,
		"db.operation.name",
		"INSERT",
	)
	requireAttribute(
		t,
		point.Attributes,
		"db.collection.name",
		"trip_status_history",
	)

	if _, ok := point.Attributes.Value(
		attribute.Key("error.type"),
	); ok {
		t.Fatal("unexpected error.type for successful operation")
	}
}

func TestMetricsExecutorExecError(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code: "23505",
	}

	executor, reader := newTestMetricsExecutor(
		t,
		&fakeExecutor{execErr: pgErr},
	)

	_, err := executor.Exec(
		context.Background(),
		"INSERT INTO trips (id) VALUES ($1)",
		"trip-id",
	)
	if !errors.Is(err, pgErr) {
		t.Fatalf("expected PostgreSQL error, got %v", err)
	}

	histogram := getDBHistogram(t, reader)

	if len(histogram.DataPoints) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(histogram.DataPoints))
	}

	point := histogram.DataPoints[0]

	requireAttribute(
		t,
		point.Attributes,
		"db.operation.name",
		"INSERT",
	)
	requireAttribute(
		t,
		point.Attributes,
		"db.collection.name",
		"trips",
	)
	requireAttribute(
		t,
		point.Attributes,
		"error.type",
		"23505",
	)
}

func TestMetricsExecutorQueryRow(t *testing.T) {
	executor, reader := newTestMetricsExecutor(
		t,
		&fakeExecutor{
			row: &fakeRow{},
		},
	)

	row := executor.QueryRow(
		context.Background(),
		"SELECT id FROM trips WHERE id = $1",
		"trip-id",
	)

	if err := row.Scan(); err != nil {
		t.Fatalf("scan row: %v", err)
	}

	histogram := getDBHistogram(t, reader)

	if len(histogram.DataPoints) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(histogram.DataPoints))
	}

	point := histogram.DataPoints[0]

	if point.Count != 1 {
		t.Fatalf("expected count 1, got %d", point.Count)
	}

	requireAttribute(
		t,
		point.Attributes,
		"db.operation.name",
		"SELECT",
	)
	requireAttribute(
		t,
		point.Attributes,
		"db.collection.name",
		"trips",
	)
}

func TestMetricsExecutorQueryRowNoRows(t *testing.T) {
	executor, reader := newTestMetricsExecutor(
		t,
		&fakeExecutor{
			row: &fakeRow{err: pgx.ErrNoRows},
		},
	)

	err := executor.QueryRow(
		context.Background(),
		"SELECT id FROM trips WHERE id = $1",
		"missing-id",
	).Scan()

	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}

	histogram := getDBHistogram(t, reader)

	if len(histogram.DataPoints) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(histogram.DataPoints))
	}

	requireAttribute(
		t,
		histogram.DataPoints[0].Attributes,
		"error.type",
		"no_rows",
	)
}

func TestMetricsExecutorQueryRowRecordedOnce(t *testing.T) {
	executor, reader := newTestMetricsExecutor(
		t,
		&fakeExecutor{
			row: &fakeRow{},
		},
	)

	row := executor.QueryRow(
		context.Background(),
		"SELECT id FROM trips WHERE id = $1",
		"trip-id",
	)

	if err := row.Scan(); err != nil {
		t.Fatalf("first scan: %v", err)
	}

	if err := row.Scan(); err != nil {
		t.Fatalf("second scan: %v", err)
	}

	histogram := getDBHistogram(t, reader)

	if len(histogram.DataPoints) != 1 {
		t.Fatalf(
			"expected 1 data point, got %d",
			len(histogram.DataPoints),
		)
	}

	if histogram.DataPoints[0].Count != 1 {
		t.Fatalf(
			"expected count 1, got %d",
			histogram.DataPoints[0].Count,
		)
	}
}

type fakeRows struct {
	pgx.Rows

	closed bool
	err    error
}

func (r *fakeRows) Close() {
	r.closed = true
}

func (r *fakeRows) Next() bool {
	return false
}

func (r *fakeRows) Err() error {
	return r.err
}

func TestMetricsExecutorQueryRecordedOnce(t *testing.T) {
	rows := &fakeRows{}

	executor, reader := newTestMetricsExecutor(
		t,
		&fakeExecutor{
			rows: rows,
		},
	)

	result, err := executor.Query(
		context.Background(),
		"SELECT id FROM trips",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	result.Close()
	result.Close()

	if !rows.closed {
		t.Fatal("underlying rows were not closed")
	}

	histogram := getDBHistogram(t, reader)

	if len(histogram.DataPoints) != 1 {
		t.Fatalf(
			"expected 1 data point, got %d",
			len(histogram.DataPoints),
		)
	}

	point := histogram.DataPoints[0]

	if point.Count != 1 {
		t.Fatalf("expected count 1, got %d", point.Count)
	}

	requireAttribute(
		t,
		point.Attributes,
		"db.operation.name",
		"SELECT",
	)

	requireAttribute(
		t,
		point.Attributes,
		"db.collection.name",
		"trips",
	)
}

func TestMetricsExecutorQueryError(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code: "42P01",
	}

	executor, reader := newTestMetricsExecutor(
		t,
		&fakeExecutor{
			queryErr: pgErr,
		},
	)

	_, err := executor.Query(
		context.Background(),
		"SELECT id FROM trips",
	)

	if !errors.Is(err, pgErr) {
		t.Fatalf("expected PostgreSQL error, got %v", err)
	}

	histogram := getDBHistogram(t, reader)

	if len(histogram.DataPoints) != 1 {
		t.Fatalf(
			"expected 1 data point, got %d",
			len(histogram.DataPoints),
		)
	}

	point := histogram.DataPoints[0]

	if point.Count != 1 {
		t.Fatalf("expected count 1, got %d", point.Count)
	}

	requireAttribute(
		t,
		point.Attributes,
		"error.type",
		"42P01",
	)
}

func TestMetricsExecutorQueryRowsError(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code: "57014",
	}

	executor, reader := newTestMetricsExecutor(
		t,
		&fakeExecutor{
			rows: &fakeRows{
				err: pgErr,
			},
		},
	)

	rows, err := executor.Query(
		context.Background(),
		"SELECT id FROM trips",
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if rows.Next() {
		t.Fatal("expected no rows")
	}

	rows.Close()

	histogram := getDBHistogram(t, reader)

	if len(histogram.DataPoints) != 1 {
		t.Fatalf(
			"expected 1 data point, got %d",
			len(histogram.DataPoints),
		)
	}

	point := histogram.DataPoints[0]

	if point.Count != 1 {
		t.Fatalf("expected count 1, got %d", point.Count)
	}

	requireAttribute(
		t,
		point.Attributes,
		"error.type",
		"57014",
	)
}
