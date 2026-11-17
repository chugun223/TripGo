package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/chugun223/TripGo/internal/infra/logger"
)

func RequestLogging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				recovered := recover()

				route := "unmatched"
				if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
					if pattern := routeContext.RoutePattern(); pattern != "" {
						route = pattern
					}
				}

				status := ww.Status()
				if recovered != nil && status == 0 {
					status = http.StatusInternalServerError
				} else if status == 0 {
					status = http.StatusOK
				}

				level := slog.LevelInfo
				if status >= http.StatusInternalServerError || recovered != nil {
					level = slog.LevelError
				}

				requestLogger := logger.WithTrace(r.Context(), log)

				requestLogger.LogAttrs(
					r.Context(),
					level,
					"http request completed",
					slog.String("method", r.Method),
					slog.String("route", route),
					slog.Int("status", status),
					slog.Float64(
						"duration_ms",
						float64(time.Since(start).Microseconds())/1000,
					),
				)

				if recovered != nil {
					panic(recovered)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
