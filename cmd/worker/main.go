package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/deus-rabbitmq-example/internal/config"
	"github.com/example/deus-rabbitmq-example/internal/messaging/rabbitmq"
	"github.com/example/deus-rabbitmq-example/internal/platform/db"
	"github.com/example/deus-rabbitmq-example/internal/platform/logging"
	"github.com/example/deus-rabbitmq-example/internal/repository/postgres"
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

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	consumer := rabbitmq.NewConsumer(
		cfg.RabbitMQURL,
		cfg.RabbitMQExchange,
		cfg.RabbitMQQueue,
		cfg.RabbitMQRoutingKey,
		postgres.NewEventRepository(pool),
		logger,
	)

	logger.Info("cargo event worker starting")
	if err := consumer.Run(ctx); err != nil {
		return fmt.Errorf("run rabbitmq consumer: %w", err)
	}
	logger.Info("cargo event worker stopped")

	return nil
}
