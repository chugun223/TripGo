package response

import (
	"encoding/json"
	"log"
	"net/http"

	domain "github.com/chugun223/TripGo/internal/domain/trip"
	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/http/domainerror"
	"github.com/chugun223/TripGo/internal/http/statuscode"
)

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response failed: %v", err)
	}
}

func Problem(
	w http.ResponseWriter,
	status int,
	code string,
	title string,
	detail string,
) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	body := api.Problem{
		Type:   "about:blank",
		Title:  title,
		Status: statuscode.ToInt32(status),
		Code:   code,
		Detail: &detail,
	}

	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write problem response failed: %v", err)
	}
}

func DomainError(w http.ResponseWriter, err error) {
	status, body := domainerror.Problem(err)

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	if encodeErr := json.NewEncoder(w).Encode(body); encodeErr != nil {
		log.Printf("write domain error failed: %v", encodeErr)
	}
}

func Trip(value domain.Trip) api.Trip {
	return api.Trip{
		Id:       value.ID,
		UserId:   value.UserID,
		DriverId: value.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  value.StartPoint.Latitude,
			Longitude: value.StartPoint.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  value.EndPoint.Latitude,
			Longitude: value.EndPoint.Longitude,
		},
		Price:      value.Price,
		Status:     api.TripStatus(value.Status),
		StartedAt:  value.StartedAt,
		FinishedAt: value.FinishedAt,
	}
}
