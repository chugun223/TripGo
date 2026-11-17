package get

import (
	"log"
	"net/http"

	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/http/response"
)

type Handler struct {
	usecase tripGetter
}

func New(usecase tripGetter) *Handler {
	return &Handler{
		usecase: usecase,
	}
}

func (h *Handler) Handle(
	w http.ResponseWriter,
	r *http.Request,
	tripID api.TripId,
) {
	result, err := h.usecase.Execute(r.Context(), tripID)
	if err != nil {
		log.Printf("get trip failed: %v", err)
		response.DomainError(w, err)
		return
	}

	response.JSON(
		w,
		http.StatusOK,
		response.Trip(result),
	)
}
