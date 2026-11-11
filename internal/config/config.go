package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains all runtime configuration, loaded from environment variables.
type Config struct {
	HTTPAddr string

	DatabaseURL string

	RabbitMQURL        string
	RabbitMQExchange   string
	RabbitMQQueue      string
	RabbitMQRoutingKey string

	LogLevel        slog.Level
	RunMigrations   bool
	ShutdownTimeout time.Duration

	HTTPReadHeaderTimeout time.Duration
	HTTPReadTimeout       time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
}

// Load reads configuration from the process environment. Defaults target the
// local docker-compose stack, so `make run` works without extra setup.
func Load() (Config, error) {
	level, err := parseLogLevel(getEnv("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	runMigrations, err := strconv.ParseBool(getEnv("RUN_MIGRATIONS", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid RUN_MIGRATIONS: %w", err)
	}

	cfg := Config{
		HTTPAddr:              getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:           getEnv("DATABASE_URL", "postgres://deus:deus@localhost:5432/deus?sslmode=disable"),
		RabbitMQURL:           getEnv("RABBITMQ_URL", "amqp://deus_user:deus_pass@localhost:5672/"),
		RabbitMQExchange:      getEnv("RABBITMQ_EXCHANGE", "cargo.events"),
		RabbitMQQueue:         getEnv("RABBITMQ_QUEUE", "cargo.events.worker"),
		RabbitMQRoutingKey:    getEnv("RABBITMQ_ROUTING_KEY", "cargo.status.changed"),
		LogLevel:              level,
		RunMigrations:         runMigrations,
		ShutdownTimeout:       10 * time.Second,
		HTTPReadHeaderTimeout: 5 * time.Second,
		HTTPReadTimeout:       10 * time.Second,
		HTTPWriteTimeout:      15 * time.Second,
		HTTPIdleTimeout:       60 * time.Second,
	}

	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL %q", value)
	}
}
