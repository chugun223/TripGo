package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
)

type CreatePositionInput struct {
	TripID      uuid.UUID
	Coordinates trip.Coordinates
	RecordedAt  time.Time
}

type StaleTrip struct {
	ID             uuid.UUID
	DriverID       uuid.UUID
	LastActivityAt time.Time
}

type StaleTripCursor struct {
	LastActivityAt time.Time
	ID             uuid.UUID
}
