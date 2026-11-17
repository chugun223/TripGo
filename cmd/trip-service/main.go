package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/chugun223/TripGo/internal/config"
	"github.com/chugun223/TripGo/internal/container"
	"github.com/chugun223/TripGo/internal/infra/logger"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application failed", "error", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	log := logger.New(cfg.Log.Level)
	slog.SetDefault(log)

	var app *container.Container
	var shutdownTracing, shutdownMetrics func(context.Context) error
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		runErr = errors.Join(runErr, shutdown(shutdownCtx, app, shutdownMetrics, shutdownTracing, log))
	}()

	shutdownTracing, err = telemetry.InitTracing(
		context.Background(),
	)
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
	}

	shutdownMetrics, err = telemetry.InitMetrics(
		context.Background(),
	)
	if err != nil {
		return fmt.Errorf("initialize metrics: %w", err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	app, err = container.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}

	if err := app.Start(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("start position workers: %w", err)
	}

	log.Info(
		"starting HTTP server",
		"address", cfg.HTTP.Addr,
	)

	serverErrors := make(chan error, 1)

	go func() {
		serverErrors <- app.Server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server failed: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received")
		return nil
	}
}

func shutdown(ctx context.Context, app *container.Container,
	metrics, tracing func(context.Context) error, log *slog.Logger,
) error {
	var result error
	if app != nil {
		if err := app.Server.Shutdown(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("shutdown HTTP server: %w", err), app.Server.Close())
		}
		log.Info("HTTP server stopped")
		result = errors.Join(result, app.Shutdown(ctx))
	}
	if metrics != nil {
		result = errors.Join(result, metrics(ctx))
	}
	if tracing != nil {
		result = errors.Join(result, tracing(ctx))
	}
	return result
}
