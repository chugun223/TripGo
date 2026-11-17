package domainerror

import (
	"errors"
	"net/http"

	"github.com/chugun223/TripGo/internal/domain/trip"
	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/http/statuscode"
)

func Problem(err error) (int, api.Problem) {
	switch {
	case errors.Is(err, trip.ErrTripNotFound):
		return newProblem(
			http.StatusNotFound,
			"trip_not_found",
			"Поездка не найдена",
			"Поездка с указанным идентификатором не существует.",
		)

	case errors.Is(err, trip.ErrTripCompleted):
		return newProblem(
			http.StatusConflict,
			"trip_completed",
			"Поездка уже завершена",
			"Нельзя повторно завершить поездку.",
		)

	case errors.Is(err, trip.ErrDriverBusy):
		return newProblem(
			http.StatusConflict,
			"driver_busy",
			"Водитель занят",
			"У водителя уже есть активная поездка.",
		)

	default:
		return newProblem(
			http.StatusInternalServerError,
			"internal_error",
			"Внутренняя ошибка",
			"Произошла внутренняя ошибка сервиса.",
		)
	}
}

func newProblem(
	status int,
	code string,
	title string,
	detail string,
) (int, api.Problem) {
	return status, api.Problem{
		Type:   "about:blank",
		Title:  title,
		Status: statuscode.ToInt32(status),
		Code:   code,
		Detail: &detail,
	}
}
