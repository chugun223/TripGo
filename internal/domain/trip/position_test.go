package trip

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPositionValidate(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		change func(*Position)
		field  string
	}{
		{name: "zero coordinates are valid"},
		{name: "boundaries", change: func(p *Position) { p.Coordinates = Coordinates{-90, 180}; p.RecordedAt = now.Add(time.Minute) }},
		{name: "empty trip", change: func(p *Position) { p.TripID = uuid.Nil }, field: "tripId"},
		{name: "latitude out of range", change: func(p *Position) { p.Coordinates.Latitude = 90.1 }, field: "coordinates.latitude"},
		{name: "longitude out of range", change: func(p *Position) { p.Coordinates.Longitude = -180.1 }, field: "coordinates.longitude"},
		{name: "not a number", change: func(p *Position) { p.Coordinates.Latitude = math.NaN() }, field: "coordinates.latitude"},
		{name: "infinity", change: func(p *Position) { p.Coordinates.Longitude = math.Inf(1) }, field: "coordinates.longitude"},
		{name: "missing time", change: func(p *Position) { p.RecordedAt = time.Time{} }, field: "recorded_at"},
		{name: "future time", change: func(p *Position) { p.RecordedAt = now.Add(time.Minute + time.Nanosecond) }, field: "recorded_at"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			position := Position{TripID: uuid.New(), RecordedAt: now}
			if tc.change != nil {
				tc.change(&position)
			}
			err := position.Validate(now)
			if tc.field == "" {
				require.NoError(t, err)
				return
			}
			var validation *ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, tc.field, validation.Field)
			require.Contains(t, validation.Error(), tc.field)
		})
	}
}
