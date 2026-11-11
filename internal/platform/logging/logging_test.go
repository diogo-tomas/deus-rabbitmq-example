package logging_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/example/deus-rabbitmq-example/internal/platform/logging"
)

func TestNew(t *testing.T) {
	logger := logging.New(slog.LevelDebug)

	assert.NotNil(t, logger)
	logger.Debug("logger constructed")
}
