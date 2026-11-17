package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/chugun223/TripGo/internal/domain/trip"
	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/http/response"
	"github.com/chugun223/TripGo/internal/model"
)

type Handler struct{ usecase positionCreator }

func New(usecase positionCreator) *Handler { return &Handler{usecase: usecase} }

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, id api.TripId) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		response.Problem(w, http.StatusBadRequest, "invalid_request", "Invalid request", "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := decode(r.Body)
	if err != nil {
		response.Problem(w, http.StatusBadRequest, "invalid_request", "Invalid request", err.Error())
		return
	}
	position, err := h.usecase.Execute(r.Context(), model.CreatePositionInput{
		TripID: id, Coordinates: trip.Coordinates{Latitude: body.Coordinates.Latitude, Longitude: body.Coordinates.Longitude},
		RecordedAt: body.RecordedAt,
	})
	if err != nil {
		response.DomainError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, response.Position(position))
}

func decode(reader io.Reader) (api.CreateTripPositionJSONRequestBody, error) {
	var body api.CreateTripPositionJSONRequestBody
	raw, err := io.ReadAll(reader)
	if err != nil {
		return body, fmt.Errorf("read position request: %w", err)
	}
	fields, err := requiredObject(raw, "coordinates", "recorded_at")
	if err != nil {
		return body, err
	}
	if _, err := requiredObject(fields["coordinates"], "latitude", "longitude"); err != nil {
		return body, fmt.Errorf("coordinates: %w", err)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return body, fmt.Errorf("invalid position JSON: %w", err)
	}
	return body, nil
}

func requiredObject(raw []byte, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("expected one JSON object: %w", err)
	}
	for _, name := range names {
		value, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("%s is required and must not be null", name)
		}
	}
	if len(fields) != len(names) {
		return nil, fmt.Errorf("unknown properties are not allowed")
	}
	return fields, nil
}
