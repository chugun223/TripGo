package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/chugun223/TripGo/internal/config"
	"github.com/chugun223/TripGo/internal/database"
	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/httpapi"
	"github.com/chugun223/TripGo/internal/repository"
	"github.com/chugun223/TripGo/internal/service"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	middleware "github.com/oapi-codegen/nethttp-middleware"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration loading failed: %v", err)
	}

	dbPool, err := database.NewPool(context.Background(), cfg.Database)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}

	log.Println("database connection established")

	tripRepository := repository.NewTripRepository(
		dbPool,
		cfg.Database.QueryTimeout,
	)

	historyRepository := repository.NewTripStatusHistoryRepository(
		dbPool,
		cfg.Database.QueryTimeout,
	)
	txManager := database.NewTxManager(dbPool)

	tripService := service.NewTripService(
		tripRepository,
		historyRepository,
		txManager,
	)

	apiHandler := httpapi.NewHandler(
		tripService,
		dbPool,
		cfg.Database.QueryTimeout,
	)

	loader := openapi3.NewLoader()

	spec, err := loader.LoadFromFile(
		"contracts/openapi/trip-service.openapi.yaml",
	)
	if err != nil {
		log.Fatalf("load OpenAPI specification: %v", err)
	}

	spec.Servers = nil

	router := chi.NewRouter()
	router.Use(maxBodySize(1 << 20))
	router.Use(middleware.OapiRequestValidatorWithOptions(
		spec,
		&middleware.Options{
			ErrorHandler: apiHandler.HandleValidationError,
		},
	))

	handler := api.HandlerWithOptions(
		apiHandler,
		api.ChiServerOptions{
			BaseRouter:       router,
			ErrorHandlerFunc: apiHandler.HandleAPIError,
		},
	)

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	log.Printf("starting server on %s", cfg.HTTP.Addr)

	go func() {
		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server failed: %v", err)
			stop()
		}
	}()

	<-ctx.Done()

	log.Println("shutdown signal received")

	if err := shutdown(
		server,
		dbPool,
		cfg.ShutdownTimeout,
	); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		return
	}

	log.Println("server stopped gracefully")
}

func maxBodySize(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func shutdown(
	server *http.Server,
	dbPool *pgxpool.Pool,
	timeout time.Duration,
) error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		timeout,
	)
	defer cancel()

	var shutdownErr error

	if err := server.Shutdown(ctx); err != nil {
		shutdownErr = errors.Join(
			shutdownErr,
			fmt.Errorf("shutdown HTTP server: %w", err),
		)

		if closeErr := server.Close(); closeErr != nil {
			shutdownErr = errors.Join(
				shutdownErr,
				fmt.Errorf("force close HTTP server: %w", closeErr),
			)
		}
	}

	poolClosed := make(chan struct{})

	go func() {
		dbPool.Close()
		close(poolClosed)
	}()

	select {
	case <-poolClosed:
		log.Println("database pool closed")

	case <-ctx.Done():
		shutdownErr = errors.Join(
			shutdownErr,
			fmt.Errorf("close database pool: %w", ctx.Err()),
		)
	}

	return shutdownErr
}
