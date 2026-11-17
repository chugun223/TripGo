package create

import (
	"errors"
	"math"

	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/model"
)

type createTripRequest struct {
	UserID     *types.UUID         `json:"user_id"`
	DriverID   *types.UUID         `json:"driver_id"`
	StartPoint *coordinatesRequest `json:"start_point"`
	EndPoint   *coordinatesRequest `json:"end_point"`
	Price      *int64              `json:"price"`
}

type coordinatesRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func (r createTripRequest) toTripData() (api.TripData, error) {
	if r.UserID == nil ||
		r.DriverID == nil ||
		r.StartPoint == nil ||
		r.EndPoint == nil ||
		r.Price == nil {
		return api.TripData{}, errors.New(
			"необходимо передать user_id, driver_id, start_point, end_point и price",
		)
	}

	if r.StartPoint.Latitude == nil ||
		r.StartPoint.Longitude == nil ||
		r.EndPoint.Latitude == nil ||
		r.EndPoint.Longitude == nil {
		return api.TripData{}, errors.New(
			"для каждой точки необходимо передать latitude и longitude",
		)
	}

	return api.TripData{
		UserId:   *r.UserID,
		DriverId: *r.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  *r.StartPoint.Latitude,
			Longitude: *r.StartPoint.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  *r.EndPoint.Latitude,
			Longitude: *r.EndPoint.Longitude,
		},
		Price: *r.Price,
	}, nil
}

func validateTripData(body api.TripData) error {
	if body.UserId == uuid.Nil {
		return errors.New("user_id должен быть ненулевым UUID")
	}

	if body.DriverId == uuid.Nil {
		return errors.New("driver_id должен быть ненулевым UUID")
	}

	if body.Price < 0 {
		return errors.New("price не может быть отрицательным")
	}

	if !validCoordinates(body.StartPoint) {
		return errors.New("start_point содержит некорректные координаты")
	}

	if !validCoordinates(body.EndPoint) {
		return errors.New("end_point содержит некорректные координаты")
	}

	return nil
}

func validCoordinates(point api.Coordinates) bool {
	if math.IsNaN(point.Latitude) || math.IsInf(point.Latitude, 0) {
		return false
	}

	if math.IsNaN(point.Longitude) || math.IsInf(point.Longitude, 0) {
		return false
	}

	return point.Latitude >= -90 &&
		point.Latitude <= 90 &&
		point.Longitude >= -180 &&
		point.Longitude <= 180
}

func toInput(body api.TripData) model.CreateTripInput {
	return model.CreateTripInput{
		UserID:   body.UserId,
		DriverID: body.DriverId,
		StartPoint: domain.Coordinates{
			Latitude:  body.StartPoint.Latitude,
			Longitude: body.StartPoint.Longitude,
		},
		EndPoint: domain.Coordinates{
			Latitude:  body.EndPoint.Latitude,
			Longitude: body.EndPoint.Longitude,
		},
		Price: body.Price,
	}
}
