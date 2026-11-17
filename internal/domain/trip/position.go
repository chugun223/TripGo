package trip

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

type Position struct {
	ID          int64
	TripID      uuid.UUID
	Coordinates Coordinates
	RecordedAt  time.Time
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (p Position) Validate(now time.Time) error {
	switch {
	case p.TripID == uuid.Nil:
		return &ValidationError{Field: "tripId", Message: "must be a nonzero UUID"}
	case !validCoordinate(p.Coordinates.Latitude, 90):
		return &ValidationError{Field: "coordinates.latitude", Message: "must be between -90 and 90"}
	case !validCoordinate(p.Coordinates.Longitude, 180):
		return &ValidationError{Field: "coordinates.longitude", Message: "must be between -180 and 180"}
	case p.RecordedAt.IsZero():
		return &ValidationError{Field: "recorded_at", Message: "is required"}
	case p.RecordedAt.After(now.Add(time.Minute)):
		return &ValidationError{Field: "recorded_at", Message: "must not be more than one minute in the future"}
	default:
		return nil
	}
}

func validCoordinate(value, limit float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= -limit && value <= limit
}
