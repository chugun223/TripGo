package create

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/http/response"
)

type Handler struct {
	usecase tripCreator
}

func New(usecase tripCreator) *Handler {
	return &Handler{
		usecase: usecase,
	}
}

func (h *Handler) Handle(
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
		response.Problem(
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
		response.Problem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Некорректное тело запроса",
			"Тело должно содержать один JSON-объект.",
		)
		return
	}

	body, err := requestBody.toTripData()
	if err != nil {
		response.Problem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Некорректные данные запроса",
			err.Error(),
		)
		return
	}

	if err := validateTripData(body); err != nil {
		response.Problem(
			w,
			http.StatusBadRequest,
			"invalid_request",
			"Некорректные данные поездки",
			err.Error(),
		)
		return
	}

	input := toInput(body)

	createdTrip, err := h.usecase.Execute(
		r.Context(),
		input,
	)
	if err != nil {
		log.Printf("create trip failed: %v", err)
		response.DomainError(w, err)
		return
	}

	w.Header().Set(
		"Location",
		"/api/v1/trips/"+createdTrip.ID.String(),
	)

	response.JSON(
		w,
		http.StatusCreated,
		response.Trip(createdTrip),
	)
}
