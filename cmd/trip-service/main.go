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
	"time"

	"github.com/joho/godotenv"

	"github.com/chugun223/TripGo/internal/config"
	"github.com/chugun223/TripGo/internal/container"
	"github.com/chugun223/TripGo/internal/infra/logger"
	"github.com/chugun223/TripGo/internal/infra/telemetry"
)

const telemetryShutdownTimeout = 5 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("application failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	log := logger.New(cfg.Log.Level)
	slog.SetDefault(log)

	shutdownTracing, err := telemetry.InitTracing(
		context.Background(),
	)
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
	}

	shutdownMetrics, err := telemetry.InitMetrics(
		context.Background(),
	)
	if err != nil {
		return fmt.Errorf("initialize metrics: %w", err)
	}

	defer func() {
		metricsCtx, cancel := context.WithTimeout(
			context.Background(),
			telemetryShutdownTimeout,
		)
		defer cancel()

		if err := shutdownMetrics(metricsCtx); err != nil {
			log.Error(
				"metrics shutdown failed",
				"error", err,
			)
		}
	}()

	defer func() {
		telemetryCtx, cancel := context.WithTimeout(
			context.Background(),
			telemetryShutdownTimeout,
		)
		defer cancel()

		if err := shutdownTracing(telemetryCtx); err != nil {
			log.Error(
				"telemetry shutdown failed",
				"error", err,
			)
		}
	}()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	app, err := container.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer app.Close()

	log.Info(
		"starting HTTP server",
		"address", cfg.HTTP.Addr,
	)

	serverErrors := make(chan error, 1)

	go func() {
		err := app.Server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout,
	)
	defer cancel()

	if err := app.Server.Shutdown(shutdownCtx); err != nil {
		log.Error(
			"server shutdown error",
			"error", err,
		)

		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	log.Info("HTTP server stopped")

	select {
	case err := <-serverErrors:
		return fmt.Errorf("HTTP server failed: %w", err)
	default:
		return nil
	}
}
