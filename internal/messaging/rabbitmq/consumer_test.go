package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

type fakeAcknowledger struct {
	acked   bool
	nacked  bool
	requeue bool
}

func (f *fakeAcknowledger) Ack(tag uint64, multiple bool) error {
	f.acked = true
	return nil
}

func (f *fakeAcknowledger) Nack(tag uint64, multiple, requeue bool) error {
	f.nacked = true
	f.requeue = requeue
	return nil
}

func (f *fakeAcknowledger) Reject(tag uint64, requeue bool) error {
	return nil
}

type fakeEventStore struct {
	events []domain.CargoStatusChanged
	err    error
}

func (f *fakeEventStore) SaveCargoStatusChanged(ctx context.Context, event domain.CargoStatusChanged) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, event)
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCargoStatusChangedEnvelopeJSON(t *testing.T) {
	event := domain.CargoStatusChanged{
		EventID:    uuid.New(),
		CargoID:    uuid.New(),
		OldStatus:  domain.CargoStatusPending,
		NewStatus:  domain.CargoStatusInTransit,
		OccurredAt: time.Now().UTC(),
	}

	body, err := json.Marshal(newCargoStatusChangedEnvelope(event))
	require.NoError(t, err)

	var decoded EventEnvelope
	require.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, event.EventID, decoded.EventID)
	assert.Equal(t, CargoStatusChangedType, decoded.Type)
	assert.Equal(t, event.CargoID, decoded.Payload.CargoID)
	assert.Equal(t, event.OldStatus, decoded.Payload.OldStatus)
	assert.Equal(t, event.NewStatus, decoded.Payload.NewStatus)
}

func TestConsumerHandleDeliveryStoresEvent(t *testing.T) {
	event := domain.CargoStatusChanged{
		EventID:    uuid.New(),
		CargoID:    uuid.New(),
		OldStatus:  domain.CargoStatusPending,
		NewStatus:  domain.CargoStatusInTransit,
		OccurredAt: time.Now().UTC(),
	}
	body, err := json.Marshal(newCargoStatusChangedEnvelope(event))
	require.NoError(t, err)

	store := &fakeEventStore{}
	acknowledger := &fakeAcknowledger{}
	consumer := NewConsumer("", "", "", "", store, testLogger())

	consumer.handleDelivery(context.Background(), amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  1,
		MessageId:    event.EventID.String(),
		Body:         body,
	})

	require.Len(t, store.events, 1)
	assert.Equal(t, event.EventID, store.events[0].EventID)
	assert.True(t, acknowledger.acked)
	assert.False(t, acknowledger.nacked)
}

func TestConsumerHandleDeliveryDeadLettersMalformedEvent(t *testing.T) {
	store := &fakeEventStore{}
	acknowledger := &fakeAcknowledger{}
	consumer := NewConsumer("", "", "", "", store, testLogger())

	consumer.handleDelivery(context.Background(), amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  1,
		Body:         []byte("{"),
	})

	assert.Empty(t, store.events)
	assert.True(t, acknowledger.nacked)
	assert.False(t, acknowledger.requeue)
}

func TestConsumerHandleDeliveryRequeuesStorageFailures(t *testing.T) {
	event := domain.CargoStatusChanged{
		EventID:    uuid.New(),
		CargoID:    uuid.New(),
		OldStatus:  domain.CargoStatusPending,
		NewStatus:  domain.CargoStatusInTransit,
		OccurredAt: time.Now().UTC(),
	}
	body, err := json.Marshal(newCargoStatusChangedEnvelope(event))
	require.NoError(t, err)

	store := &fakeEventStore{err: errors.New("database unavailable")}
	acknowledger := &fakeAcknowledger{}
	consumer := NewConsumer("", "", "", "", store, testLogger())

	consumer.handleDelivery(context.Background(), amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  1,
		Body:         body,
	})

	assert.Empty(t, store.events)
	assert.True(t, acknowledger.nacked)
	assert.True(t, acknowledger.requeue)
}

func TestConsumerHandleDeliveryAcksUnsupportedEvents(t *testing.T) {
	body, err := json.Marshal(EventEnvelope{
		EventID:    uuid.New(),
		Type:       "cargo.other",
		OccurredAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	store := &fakeEventStore{}
	acknowledger := &fakeAcknowledger{}
	consumer := NewConsumer("", "", "", "", store, testLogger())

	consumer.handleDelivery(context.Background(), amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  1,
		Body:         body,
	})

	assert.Empty(t, store.events)
	assert.True(t, acknowledger.acked)
	assert.False(t, acknowledger.nacked)
}
