package loggertest

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func Capture(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buffer bytes.Buffer

	previous := slog.Default()

	testLogger := slog.New(
		slog.NewJSONHandler(&buffer, nil),
	).With(
		slog.String("service", "trip-service"),
	)

	slog.SetDefault(testLogger)

	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	return &buffer
}

func Records(t *testing.T, buffer *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any

	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))

	for decoder.More() {
		var record map[string]any

		if err := decoder.Decode(&record); err != nil {
			t.Fatalf("decode log record: %v", err)
		}

		records = append(records, record)
	}

	return records
}
