package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"

	"github.com/chugun223/TripGo/internal/config"
	"github.com/chugun223/TripGo/internal/database"
	api "github.com/chugun223/TripGo/internal/generated"
	"github.com/chugun223/TripGo/internal/httpapi"
	"github.com/chugun223/TripGo/internal/repository"
	"github.com/chugun223/TripGo/internal/service"
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
	defer dbPool.Close()

	log.Println("database connection established")

	tripRepository := repository.NewTripRepository(dbPool)
	historyRepository := repository.NewTripStatusHistoryRepository(dbPool)
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

	router := chi.NewRouter()
	handler := api.HandlerFromMux(apiHandler, router)

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

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
