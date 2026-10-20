package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/chugun223/TripGo/internal/domain"
	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	tripService  *service.TripService
	dbPool       *pgxpool.Pool
	queryTimeout time.Duration
}

func NewHandler(
	tripService *service.TripService,
	dbPool *pgxpool.Pool,
	queryTimeout time.Duration,
) *Handler {
	return &Handler{
		tripService:  tripService,
		dbPool:       dbPool,
		queryTimeout: queryTimeout,
	}
}

func (h *Handler) CreateTrip(
	w http.ResponseWriter,
	r *http.Request,
	params api.CreateTripParams,
) {
	_ = params

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var requestBody createTripRequest
	if err := decoder.Decode(&requestBody); err != nil {
		writeProblem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Некорректный JSON",
			"Проверьте тело запроса.",
		)
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeProblem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Некорректное тело запроса",
			"Тело должно содержать один JSON-объект.",
		)
		return
	}

	if requestBody.UserID == nil ||
		requestBody.DriverID == nil ||
		requestBody.StartPoint == nil ||
		requestBody.EndPoint == nil ||
		requestBody.Price == nil {
		writeProblem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Отсутствуют обязательные поля",
			"Необходимо передать user_id, driver_id, start_point, end_point и price.",
		)
		return
	}

	if requestBody.StartPoint.Latitude == nil ||
		requestBody.StartPoint.Longitude == nil ||
		requestBody.EndPoint.Latitude == nil ||
		requestBody.EndPoint.Longitude == nil {
		writeProblem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Отсутствуют координаты",
			"Для каждой точки необходимо передать latitude и longitude.",
		)
		return
	}

	body := api.TripData{
		UserId:   *requestBody.UserID,
		DriverId: *requestBody.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  *requestBody.StartPoint.Latitude,
			Longitude: *requestBody.StartPoint.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  *requestBody.EndPoint.Latitude,
			Longitude: *requestBody.EndPoint.Longitude,
		},
		Price: *requestBody.Price,
	}

	if err := validateTripData(body); err != nil {
		writeProblem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Некорректные данные поездки",
			err.Error(),
		)
		return
	}

	trip := domain.Trip{
		ID:       uuid.New(),
		UserID:   uuid.UUID(body.UserId),
		DriverID: uuid.UUID(body.DriverId),
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

	createdTrip, err := h.tripService.Create(r.Context(), trip)
	if err != nil {
		if errors.Is(err, domain.ErrDriverBusy) {
			writeProblem(
				w,
				http.StatusConflict,
				"driver_busy",
				"Водитель занят",
				"У водителя уже есть активная поездка.",
			)
			return
		}

		log.Printf("create trip failed: %v", err)
		writeProblem(
			w,
			http.StatusInternalServerError,
			"internal_error",
			"Внутренняя ошибка",
			"Не удалось создать поездку.",
		)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+createdTrip.ID.String())
	writeJSON(w, http.StatusCreated, toAPITrip(createdTrip))
}

func (h *Handler) GetTrip(
	w http.ResponseWriter,
	r *http.Request,
	tripID api.TripId,
) {
	id := uuid.UUID(tripID)

	trip, err := h.tripService.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrTripNotFound) {
			writeProblem(
				w,
				http.StatusNotFound,
				"trip_not_found",
				"Поездка не найдена",
				"Поездка с указанным идентификатором не существует.",
			)
			return
		}

		log.Printf("get trip failed: %v", err)
		writeProblem(
			w,
			http.StatusInternalServerError,
			"internal_error",
			"Внутренняя ошибка",
			"Не удалось получить поездку.",
		)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(trip))
}

func (h *Handler) FinishTrip(
	w http.ResponseWriter,
	r *http.Request,
	tripID api.TripId,
) {
	id := uuid.UUID(tripID)

	trip, err := h.tripService.Finish(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrTripNotFound) {
			writeProblem(
				w,
				http.StatusNotFound,
				"trip_not_found",
				"Поездка не найдена",
				"Поездка с указанным идентификатором не существует.",
			)
			return
		}

		if errors.Is(err, domain.ErrTripCompleted) {
			writeProblem(
				w,
				http.StatusConflict,
				"trip_completed",
				"Поездка уже завершена",
				"Нельзя повторно завершить поездку.",
			)
			return
		}

		log.Printf("finish trip failed: %v", err)
		writeProblem(
			w,
			http.StatusInternalServerError,
			"internal_error",
			"Внутренняя ошибка",
			"Не удалось завершить поездку.",
		)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(trip))
}

func (h *Handler) Health(
	w http.ResponseWriter,
	r *http.Request,
) {
	writeJSON(w, http.StatusOK, api.HealthResponse{
		Status: api.Ok,
	})
}

func (h *Handler) Ready(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx, cancel := contextWithTimeout(r, h.queryTimeout)
	defer cancel()

	if err := h.dbPool.Ping(ctx); err != nil {
		log.Printf("readiness check failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{
			Status: api.Unavailable,
		})
		return
	}

	writeJSON(w, http.StatusOK, api.HealthResponse{
		Status: api.Ok,
	})
}

func contextWithTimeout(
	r *http.Request,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), timeout)
}

type createTripRequest struct {
	UserID     *openapi_types.UUID `json:"user_id"`
	DriverID   *openapi_types.UUID `json:"driver_id"`
	StartPoint *coordinatesRequest `json:"start_point"`
	EndPoint   *coordinatesRequest `json:"end_point"`
	Price      *int64              `json:"price"`
}

type coordinatesRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func validateTripData(body api.TripData) error {
	if uuid.UUID(body.UserId) == uuid.Nil {
		return errors.New("user_id должен быть ненулевым UUID")
	}

	if uuid.UUID(body.DriverId) == uuid.Nil {
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

func toAPITrip(trip domain.Trip) api.Trip {
	return api.Trip{
		Id:       openapi_types.UUID(trip.ID),
		UserId:   openapi_types.UUID(trip.UserID),
		DriverId: openapi_types.UUID(trip.DriverID),
		StartPoint: api.Coordinates{
			Latitude:  trip.StartPoint.Latitude,
			Longitude: trip.StartPoint.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  trip.EndPoint.Latitude,
			Longitude: trip.EndPoint.Longitude,
		},
		Price:      trip.Price,
		Status:     api.TripStatus(trip.Status),
		StartedAt:  trip.StartedAt,
		FinishedAt: trip.FinishedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response failed: %v", err)
	}
}

func writeProblem(
	w http.ResponseWriter,
	status int,
	code string,
	title string,
	detail string,
) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	problem := api.Problem{
		Type:   "about:blank",
		Title:  title,
		Status: int32(status),
		Code:   code,
		Detail: &detail,
	}

	if err := json.NewEncoder(w).Encode(problem); err != nil {
		log.Printf("write problem response failed: %v", err)
	}
}

var _ api.ServerInterface = (*Handler)(nil)
