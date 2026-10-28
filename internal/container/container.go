package container

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/chugun223/TripGo/internal/config"
	api "github.com/chugun223/TripGo/internal/generated"
	httpapi "github.com/chugun223/TripGo/internal/http"
	"github.com/chugun223/TripGo/internal/http/middleware"
	createhandler "github.com/chugun223/TripGo/internal/http/trip/create"
	finishhandler "github.com/chugun223/TripGo/internal/http/trip/finish"
	gethandler "github.com/chugun223/TripGo/internal/http/trip/get"
	"github.com/chugun223/TripGo/internal/infra/observability"
	"github.com/chugun223/TripGo/internal/infra/postgres"
	"github.com/chugun223/TripGo/internal/infra/postgres/transaction"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
	triprepo "github.com/chugun223/TripGo/internal/repository/trip"
	createusecase "github.com/chugun223/TripGo/internal/usecase/trip/create"
	finishusecase "github.com/chugun223/TripGo/internal/usecase/trip/finish"
	getusecase "github.com/chugun223/TripGo/internal/usecase/trip/get"
)

type Container struct {
	Server *http.Server

	dbPool      *pgxpool.Pool
	tripMetrics *telemetry.TripMetrics
}

func New(
	ctx context.Context,
	cfg config.Config,
) (*Container, error) {
	dbPool, err := postgres.NewPool(ctx, cfg.Database)
	log := slog.Default()
	if err != nil {
		return nil, fmt.Errorf(
			"initialize database: %w",
			err,
		)
	}

	dbMetrics, err := telemetry.NewDBMetrics()
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("initialize database metrics: %w", err)
	}

	tripRepository := triprepo.NewTripRepository(dbPool, dbMetrics)
	historyRepository := triprepo.NewTripStatusHistoryRepository(dbPool, dbMetrics)
	txManager := transaction.NewTxManager(dbPool)

	tripMetrics, err := telemetry.NewTripMetrics()
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("initialize trip metrics: %w", err)
	}

	if err := tripMetrics.RegisterActive(
		tripRepository,
		cfg.Database.QueryTimeout,
	); err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("register active trip metrics: %w", err)
	}

	createUC := createusecase.New(
		tripRepository,
		historyRepository,
		txManager,
	)

	getUC := getusecase.New(tripRepository)

	finishUC := finishusecase.New(
		tripRepository,
		historyRepository,
		txManager,
	)

	observedCreateUC := observability.NewCreate(
		createUC,
		tripMetrics,
		log,
	)

	observedFinishUC := observability.NewFinish(
		finishUC,
		tripMetrics,
		log,
	)

	createHandler := createhandler.New(observedCreateUC)
	getHandler := gethandler.New(getUC)
	finishHandler := finishhandler.New(observedFinishUC)

	apiHandler := httpapi.NewHandler(
		createHandler,
		getHandler,
		finishHandler,
		dbPool,
		cfg.Database.QueryTimeout,
	)

	router := chi.NewRouter()

	httpMetrics, err := telemetry.NewHTTPMetrics()
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("initialize HTTP metrics: %w", err)
	}

	router.Use(middleware.RequestLogging(log))
	router.Use(middleware.Metrics(httpMetrics, router))

	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				routeCtx := chi.RouteContext(r.Context())
				if routeCtx == nil {
					return
				}

				route := routeCtx.RoutePattern()
				if route != "" {
					trace.SpanFromContext(r.Context()).SetName(
						r.Method + " " + route,
					)
				}
			}()

			next.ServeHTTP(w, r)
		})
	})

	apiHandlerWithRoutes := api.HandlerFromMux(
		apiHandler,
		router,
	)

	tracedHandler := otelhttp.NewHandler(
		apiHandlerWithRoutes,
		"HTTP request",
	)

	handler := middleware.Recover(log)(tracedHandler)

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	return &Container{
		Server:      server,
		dbPool:      dbPool,
		tripMetrics: tripMetrics,
	}, nil
}

func (c *Container) Close() {
	if err := c.tripMetrics.Close(); err != nil {
		slog.Error("unregister trip metrics", slog.Any("error", err))
	}

	c.dbPool.Close()
}
