//go:build integration

package rabbitmq

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

type recordingStore struct {
	mu     sync.Mutex
	events []domain.CargoStatusChanged
}

func (s *recordingStore) SaveCargoStatusChanged(ctx context.Context, event domain.CargoStatusChanged) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, event)

	return nil
}

func (s *recordingStore) snapshot() []domain.CargoStatusChanged {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]domain.CargoStatusChanged(nil), s.events...)
}

func rabbitMQURL(t *testing.T) string {
	t.Helper()

	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("TEST_RABBITMQ_URL is not set")
	}

	return url
}

func TestPublisherAndConsumerRoundTrip(t *testing.T) {
	url := rabbitMQURL(t)
	suffix := uuid.NewString()
	exchange := "test.cargo.events." + suffix
	queue := "test.cargo.events.worker." + suffix
	routingKey := "cargo.status.changed." + suffix

	publisher, err := NewPublisher(url, exchange, queue, routingKey, testLogger())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, publisher.Close())
	})
	t.Cleanup(func() {
		_, _ = publisher.channel.QueueDelete(queue, false, false, false)
		_ = publisher.channel.ExchangeDelete(exchange, false, false)
	})

	event := domain.CargoStatusChanged{
		EventID:    uuid.New(),
		CargoID:    uuid.New(),
		OldStatus:  domain.CargoStatusPending,
		NewStatus:  domain.CargoStatusInTransit,
		OccurredAt: time.Now().UTC(),
	}
	require.NoError(t, publisher.PublishCargoStatusChanged(context.Background(), event))

	store := &recordingStore{}
	consumer := NewConsumer(url, exchange, queue, routingKey, store, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- consumer.Run(ctx)
	}()

	require.Eventually(t, func() bool {
		return len(store.snapshot()) == 1
	}, 10*time.Second, 50*time.Millisecond)

	cancel()

	select {
	case err := <-runErr:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("consumer did not stop after context cancellation")
	}

	events := store.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, event.EventID, events[0].EventID)
	assert.Equal(t, event.CargoID, events[0].CargoID)
	assert.Equal(t, event.OldStatus, events[0].OldStatus)
	assert.Equal(t, event.NewStatus, events[0].NewStatus)
}

func TestNewPublisherRejectsInvalidURL(t *testing.T) {
	_, err := NewPublisher("amqp://127.0.0.1:1/", "exchange", "queue", "key", testLogger())

	require.Error(t, err)
}
