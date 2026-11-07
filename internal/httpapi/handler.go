package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
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
	var body api.CreateTripJSONRequestBody

	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&body); err != nil {
		writeProblem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Некорректный JSON",
			"Проверьте тело запроса.",
		)
		return
	}

	if uuid.UUID(body.UserId) == uuid.Nil ||
		uuid.UUID(body.DriverId) == uuid.Nil {
		writeProblem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Некорректные идентификаторы",
			"user_id и driver_id должны быть ненулевыми UUID.",
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

	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	createdTrip, err := h.tripService.Create(ctx, trip)
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

	w.Header().Set(
		"Location",
		"/api/v1/trips/"+createdTrip.ID.String(),
	)

	writeJSON(w, http.StatusCreated, toAPITrip(createdTrip))
}

func (h *Handler) GetTrip(
	w http.ResponseWriter,
	r *http.Request,
	tripID api.TripId,
) {
	id := uuid.UUID(tripID)

	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	trip, err := h.tripService.GetByID(ctx, id)
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

	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	trip, err := h.tripService.Finish(ctx, id)
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

func (h *Handler) HandleValidationError(
	w http.ResponseWriter,
	message string,
	statusCode int,
) {
	writeProblem(
		w,
		statusCode,
		"invalid_request",
		"Некорректный запрос",
		message,
	)
}

func (h *Handler) HandleAPIError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	log.Printf("API request validation failed: %v", err)

	writeProblem(
		w,
		http.StatusBadRequest,
		"invalid_request",
		"Некорректный запрос",
		"Параметры запроса не соответствуют API-контракту.",
	)
}

var _ api.ServerInterface = (*Handler)(nil)
