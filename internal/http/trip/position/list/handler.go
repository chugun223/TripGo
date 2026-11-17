package list

import (
	"net/http"

	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/http/response"
)

type Handler struct{ usecase positionLister }

func New(usecase positionLister) *Handler { return &Handler{usecase: usecase} }

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, id api.TripId) {
	positions, err := h.usecase.Execute(r.Context(), id)
	if err != nil {
		response.DomainError(w, err)
		return
	}
	items := make([]api.TripPosition, 0, len(positions))
	for _, position := range positions {
		items = append(items, response.Position(position))
	}
	response.JSON(w, http.StatusOK, api.TripPositionsResponse{Positions: items})
}
