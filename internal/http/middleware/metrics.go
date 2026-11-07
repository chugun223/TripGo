package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

func Metrics(
	instruments *telemetry.HTTPMetrics,
	routes chi.Routes,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			route := "unmatched"
			routeCtx := chi.NewRouteContext()

			if routes.Match(routeCtx, r.Method, r.URL.Path) {
				if pattern := routeCtx.RoutePattern(); pattern != "" {
					route = pattern
				}
			}

			methodAttr := attribute.String("http.request.method", r.Method)
			routeAttr := attribute.String("http.route", route)

			activeOptions := metric.WithAttributes(methodAttr, routeAttr)

			instruments.ActiveRequests.Add(ctx, 1, activeOptions)
			defer instruments.ActiveRequests.Add(ctx, -1, activeOptions)

			start := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				recovered := recover()

				status := ww.Status()
				if recovered != nil && status == 0 {
					status = http.StatusInternalServerError
				} else if status == 0 {
					status = http.StatusOK
				}

				instruments.RequestDuration.Record(
					ctx,
					time.Since(start).Seconds(),
					metric.WithAttributes(
						methodAttr,
						routeAttr,
						attribute.Int("http.response.status_code", status),
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
