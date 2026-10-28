package http

import (
	"context"
	"log"
	"net/http"
	"time"

	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/http/response"
	createhandler "github.com/chugun223/TripGo/internal/http/trip/create"
	finishhandler "github.com/chugun223/TripGo/internal/http/trip/finish"
	gethandler "github.com/chugun223/TripGo/internal/http/trip/get"
)

type databasePinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	create *createhandler.Handler
	get    *gethandler.Handler
	finish *finishhandler.Handler

	dbPinger     databasePinger
	queryTimeout time.Duration
}

func NewHandler(
	create *createhandler.Handler,
	get *gethandler.Handler,
	finish *finishhandler.Handler,
	dbPinger databasePinger,
	queryTimeout time.Duration,
) *Handler {
	return &Handler{
		create:       create,
		get:          get,
		finish:       finish,
		dbPinger:     dbPinger,
		queryTimeout: queryTimeout,
	}
}

func (h *Handler) CreateTrip(
	w http.ResponseWriter,
	r *http.Request,
	params api.CreateTripParams,
) {
	h.create.Handle(w, r, params)
}

func (h *Handler) GetTrip(
	w http.ResponseWriter,
	r *http.Request,
	id api.TripId,
) {
	h.get.Handle(w, r, id)
}

func (h *Handler) FinishTrip(
	w http.ResponseWriter,
	r *http.Request,
	id api.TripId,
) {
	h.finish.Handle(w, r, id)
}

func (h *Handler) Health(
	w http.ResponseWriter,
	r *http.Request,
) {
	response.JSON(w, http.StatusOK, api.HealthResponse{
		Status: api.Ok,
	})
}

func (h *Handler) Ready(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx, cancel := context.WithTimeout(
		r.Context(),
		h.queryTimeout,
	)
	defer cancel()

	if err := h.dbPinger.Ping(ctx); err != nil {
		log.Printf("readiness check failed: %v", err)

		response.JSON(
			w,
			http.StatusServiceUnavailable,
			api.HealthResponse{
				Status: api.Unavailable,
			},
		)
		return
	}

	response.JSON(w, http.StatusOK, api.HealthResponse{
		Status: api.Ok,
	})
}

var _ api.ServerInterface = (*Handler)(nil)
