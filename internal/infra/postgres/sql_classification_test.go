package postgres

import "testing"

func TestClassifySQL(t *testing.T) {
	const tripsTable = "trips"

	tests := []struct {
		name       string
		sql        string
		operation  string
		collection string
	}{
		{
			name:       "insert trips",
			sql:        "INSERT INTO trips (id) VALUES ($1)",
			operation:  "INSERT",
			collection: tripsTable,
		},
		{
			name:       "select trips",
			sql:        "SELECT id FROM trips WHERE id = $1",
			operation:  "SELECT",
			collection: tripsTable,
		},
		{
			name:       "update trips",
			sql:        "UPDATE trips SET status = $1 WHERE id = $2",
			operation:  "UPDATE",
			collection: tripsTable,
		},
		{
			name:       "insert history",
			sql:        "INSERT INTO trip_status_history (trip_id) VALUES ($1)",
			operation:  "INSERT",
			collection: "trip_status_history",
		},
		{
			name:       "empty query",
			sql:        "",
			operation:  "UNKNOWN",
			collection: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operation, collection := classifySQL(tt.sql)

			if operation != tt.operation {
				t.Errorf(
					"operation = %q, want %q",
					operation,
					tt.operation,
				)
			}

			if collection != tt.collection {
				t.Errorf(
					"collection = %q, want %q",
					collection,
					tt.collection,
				)
			}
		})
	}
}
