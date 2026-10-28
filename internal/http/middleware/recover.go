package middleware

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/chugun223/TripGo/internal/http/domainerror"
	"github.com/chugun223/TripGo/internal/infra/logger"
)

func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			ww := chimiddleware.NewWrapResponseWriter(
				w,
				r.ProtoMajor,
			)

			defer func() {
				if recovered := recover(); recovered != nil {
					requestLogger := logger.WithTrace(
						r.Context(),
						log,
					)

					requestLogger.ErrorContext(
						r.Context(),
						"HTTP handler panic",
						slog.Any("panic", recovered),
						slog.String("stack", string(debug.Stack())),
					)

					if ww.Status() != 0 {
						return
					}

					status, problem := domainerror.Problem(
						errors.New("internal server error"),
					)

					ww.Header().Set(
						"Content-Type",
						"application/problem+json",
					)
					ww.WriteHeader(status)

					if err := json.NewEncoder(ww).Encode(problem); err != nil {
						requestLogger.ErrorContext(
							r.Context(),
							"write panic response",
							slog.Any("error", err),
						)
					}
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
