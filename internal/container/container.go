package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"

	"github.com/chugun223/TripGo/internal/config"
	"github.com/chugun223/TripGo/internal/gateway/push"
	api "github.com/chugun223/TripGo/internal/generated"
	httpapi "github.com/chugun223/TripGo/internal/http"
	"github.com/chugun223/TripGo/internal/http/middleware"
	createhandler "github.com/chugun223/TripGo/internal/http/trip/create"
	finishhandler "github.com/chugun223/TripGo/internal/http/trip/finish"
	gethandler "github.com/chugun223/TripGo/internal/http/trip/get"
	createpositionhandler "github.com/chugun223/TripGo/internal/http/trip/position/create"
	listpositionshandler "github.com/chugun223/TripGo/internal/http/trip/position/list"
	"github.com/chugun223/TripGo/internal/infra/observability"
	"github.com/chugun223/TripGo/internal/infra/postgres"
	"github.com/chugun223/TripGo/internal/infra/postgres/transaction"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
	positionrepo "github.com/chugun223/TripGo/internal/repository/position"
	triprepo "github.com/chugun223/TripGo/internal/repository/trip"
	createusecase "github.com/chugun223/TripGo/internal/usecase/trip/create"
	finishusecase "github.com/chugun223/TripGo/internal/usecase/trip/finish"
	getusecase "github.com/chugun223/TripGo/internal/usecase/trip/get"
	createpositionusecase "github.com/chugun223/TripGo/internal/usecase/trip/position/create"
	listpositionsusecase "github.com/chugun223/TripGo/internal/usecase/trip/position/list"
	"github.com/chugun223/TripGo/internal/worker/positions"
)

type Container struct {
	Server *http.Server

	dbPool          *pgxpool.Pool
	tripMetrics     *telemetry.TripMetrics
	positionMetrics *telemetry.PositionMetrics
	positions       *positions.Pool
	push            *push.Gateway
	closeOnce       sync.Once
	closeDone       chan struct{}
	closeErr        error
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
	initialized := false
	defer func() {
		if !initialized {
			_ = tripMetrics.Close()
		}
	}()

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

	positionMetrics, err := telemetry.NewPositionMetrics()
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("initialize position metrics: %w", err)
	}
	pushGateway := push.New(cfg.Push.HTTPURL, cfg.Push.RequestTimeout)
	positionPool, err := positions.New(tripRepository, pushGateway, positions.Settings{
		CheckInterval: cfg.Positions.CheckInterval, StaleAfter: cfg.Positions.StaleAfter,
		Workers: cfg.Positions.Workers, QueueSize: cfg.Positions.QueueSize,
		BatchSize:    cfg.Positions.BatchSize,
		QueryTimeout: cfg.Database.QueryTimeout, RequestTimeout: cfg.Push.RequestTimeout,
	}, positionMetrics, log)
	if err != nil {
		pushGateway.Close()
		dbPool.Close()
		return nil, fmt.Errorf("initialize position pool: %w", err)
	}
	defer func() {
		if !initialized {
			_ = positionMetrics.Close()
			pushGateway.Close()
		}
	}()
	if err := positionMetrics.RegisterPool(positionPool); err != nil {
		dbPool.Close()
		return nil, err
	}
	positionRepository := positionrepo.New(dbPool, dbMetrics)
	createPositionUC := createpositionusecase.New(tripRepository, positionRepository, txManager,
		positionPool, time.Now, cfg.Database.QueryTimeout)
	listPositionsUC := listpositionsusecase.New(tripRepository, positionRepository, cfg.Database.QueryTimeout)
	positionHandlers := httpapi.PositionHandlers{
		Create: createpositionhandler.New(observability.NewCreatePosition(createPositionUC, positionMetrics, log)),
		List:   listpositionshandler.New(observability.NewListPositions(listPositionsUC, log)),
	}

	apiHandler := httpapi.NewHandler(
		createHandler,
		getHandler,
		finishHandler,
		dbPool,
		cfg.Database.QueryTimeout,
		positionHandlers,
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
					span := trace.SpanFromContext(r.Context())

					span.SetName(r.Method + " " + route)

					span.SetAttributes(
						attribute.String("http.route", route),
					)
				}
			}()

			next.ServeHTTP(w, r)
		})
	})

	apiHandlerWithRoutes := api.HandlerWithOptions(
		apiHandler,
		api.ChiServerOptions{BaseRouter: router, ErrorHandlerFunc: httpapi.PositionParameterError},
	)

	recoveredHandler := middleware.Recover(log)(
		apiHandlerWithRoutes,
	)

	handler := otelhttp.NewHandler(
		recoveredHandler,
		"HTTP request",
		otelhttp.WithMeterProvider(noop.NewMeterProvider()),
	)

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	initialized = true
	return &Container{
		Server:          server,
		dbPool:          dbPool,
		tripMetrics:     tripMetrics,
		positionMetrics: positionMetrics, positions: positionPool, push: pushGateway,
		closeDone: make(chan struct{}),
	}, nil
}

func (c *Container) Start(ctx context.Context) error {
	return c.positions.Start(ctx)
}

func (c *Container) Shutdown(ctx context.Context) error {
	workerErr := c.positions.Shutdown(ctx)
	c.closeOnce.Do(func() {
		go func() {
			c.push.Close()
			c.closeErr = errors.Join(c.positionMetrics.Close(), c.tripMetrics.Close())
			c.dbPool.Close()
			close(c.closeDone)
		}()
	})
	select {
	case <-c.closeDone:
		return errors.Join(workerErr, c.closeErr)
	case <-ctx.Done():
		return errors.Join(workerErr, fmt.Errorf("close application resources: %w", ctx.Err()))
	}
}
