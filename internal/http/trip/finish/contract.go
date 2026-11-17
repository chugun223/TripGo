package finish

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
)

type tripFinisher interface {
	Execute(ctx context.Context, id uuid.UUID) (domain.Trip, error)
}
