package model

import (
	"github.com/google/uuid"

	"github.com/chugun223/TripGo/internal/domain/trip"
)

type CreateTripInput struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint trip.Coordinates
	EndPoint   trip.Coordinates
	Price      int64
}
