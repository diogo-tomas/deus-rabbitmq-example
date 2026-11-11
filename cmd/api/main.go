package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/deus-rabbitmq-example/internal/config"
	"github.com/example/deus-rabbitmq-example/internal/messaging/rabbitmq"
	"github.com/example/deus-rabbitmq-example/internal/platform/db"
	"github.com/example/deus-rabbitmq-example/internal/platform/logging"
	"github.com/example/deus-rabbitmq-example/internal/repository/postgres"
	"github.com/example/deus-rabbitmq-example/internal/service"
	"github.com/example/deus-rabbitmq-example/internal/transport/httpapi"
	"github.com/example/deus-rabbitmq-example/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := logging.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.RunMigrations {
		if err := migrations.Up(cfg.DatabaseURL); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
		logger.Info("database migrations applied")
	}

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	publisher, err := rabbitmq.NewPublisher(
		cfg.RabbitMQURL,
		cfg.RabbitMQExchange,
		cfg.RabbitMQQueue,
		cfg.RabbitMQRoutingKey,
		logger,
	)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer func() {
		if err := publisher.Close(); err != nil {
			logger.Warn("close rabbitmq publisher", "error", err)
		}
	}()

	cargoRepo := postgres.NewCargoRepository(pool)
	vesselRepo := postgres.NewVesselRepository(pool)
	trackingRepo := postgres.NewTrackingRepository(pool)

	cargoService := service.NewCargoService(cargoRepo, vesselRepo, publisher, logger)
	vesselService := service.NewVesselService(vesselRepo)
	trackingService := service.NewTrackingService(trackingRepo, cargoRepo)

	router := httpapi.NewRouter(
		httpapi.NewCargoHandler(cargoService, logger),
		httpapi.NewVesselHandler(vesselService, logger),
		httpapi.NewTrackingHandler(trackingService, logger),
		pool,
		logger,
	)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	logger.Info("http server stopped")

	return nil
}
