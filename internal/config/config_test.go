package config_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/deus-rabbitmq-example/internal/config"
)

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	value, ok := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/app")
	t.Setenv("RABBITMQ_URL", "amqp://guest:guest@rabbit:5672/")
	t.Setenv("RABBITMQ_EXCHANGE", "test.events")
	t.Setenv("RABBITMQ_QUEUE", "test.queue")
	t.Setenv("RABBITMQ_ROUTING_KEY", "test.key")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("RUN_MIGRATIONS", "false")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.HTTPAddr)
	assert.Equal(t, "postgres://user:pass@db:5432/app", cfg.DatabaseURL)
	assert.Equal(t, "amqp://guest:guest@rabbit:5672/", cfg.RabbitMQURL)
	assert.Equal(t, "test.events", cfg.RabbitMQExchange)
	assert.Equal(t, "test.queue", cfg.RabbitMQQueue)
	assert.Equal(t, "test.key", cfg.RabbitMQRoutingKey)
	assert.Equal(t, slog.LevelDebug, cfg.LogLevel)
	assert.False(t, cfg.RunMigrations)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
}

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{
		"HTTP_ADDR",
		"DATABASE_URL",
		"RABBITMQ_URL",
		"RABBITMQ_EXCHANGE",
		"RABBITMQ_QUEUE",
		"RABBITMQ_ROUTING_KEY",
		"LOG_LEVEL",
		"RUN_MIGRATIONS",
	} {
		unsetEnv(t, key)
	}

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.HTTPAddr)
	assert.Equal(t, "postgres://deus:deus@localhost:5432/deus?sslmode=disable", cfg.DatabaseURL)
	assert.Equal(t, "amqp://deus_user:deus_pass@localhost:5672/", cfg.RabbitMQURL)
	assert.Equal(t, "cargo.events", cfg.RabbitMQExchange)
	assert.Equal(t, "cargo.events.worker", cfg.RabbitMQQueue)
	assert.Equal(t, "cargo.status.changed", cfg.RabbitMQRoutingKey)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
	assert.True(t, cfg.RunMigrations)
}

func TestLoadLogLevels(t *testing.T) {
	tests := []struct {
		value   string
		want    slog.Level
		wantErr bool
	}{
		{value: "debug", want: slog.LevelDebug},
		{value: "info", want: slog.LevelInfo},
		{value: "warn", want: slog.LevelWarn},
		{value: "warning", want: slog.LevelWarn},
		{value: "error", want: slog.LevelError},
		{value: "verbose", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tt.value)
			t.Setenv("DATABASE_URL", "postgres://example")

			cfg, err := config.Load()
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.LogLevel)
		})
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Run("run migrations", func(t *testing.T) {
		t.Setenv("RUN_MIGRATIONS", "sometimes")
		t.Setenv("DATABASE_URL", "postgres://example")

		_, err := config.Load()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "RUN_MIGRATIONS")
	})

	t.Run("empty database url", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "   ")

		_, err := config.Load()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "DATABASE_URL")
	})
}
