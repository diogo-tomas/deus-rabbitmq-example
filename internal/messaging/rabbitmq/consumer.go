package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

// EventStore persists consumed events.
type EventStore interface {
	SaveCargoStatusChanged(ctx context.Context, event domain.CargoStatusChanged) error
}

// Consumer reads cargo events from RabbitMQ and stores them in Postgres.
type Consumer struct {
	url        string
	exchange   string
	queue      string
	routingKey string
	store      EventStore
	logger     *slog.Logger
}

func NewConsumer(url, exchange, queue, routingKey string, store EventStore, logger *slog.Logger) *Consumer {
	return &Consumer{
		url:        url,
		exchange:   exchange,
		queue:      queue,
		routingKey: routingKey,
		store:      store,
		logger:     logger,
	}
}

// Run consumes until the context is canceled, reconnecting with exponential
// backoff when the broker or channel fails.
func (c *Consumer) Run(ctx context.Context) error {
	const maxBackoff = 30 * time.Second
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return nil
		}

		err := c.consume(ctx)
		if err == nil || errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return nil
		}

		if c.logger != nil {
			c.logger.ErrorContext(ctx, "rabbitmq consumer disconnected", "error", err, "retry_in", backoff.String())
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (c *Consumer) consume(ctx context.Context) error {
	conn, err := amqp.DialConfig(c.url, amqp.Config{Dial: amqp.DefaultDial(10 * time.Second)})
	if err != nil {
		return fmt.Errorf("connect to rabbitmq: %w", err)
	}
	defer conn.Close()

	channel, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	defer channel.Close()

	if err := declareTopology(channel, c.exchange, c.queue, c.routingKey); err != nil {
		return err
	}
	if err := channel.Qos(10, 0, false); err != nil {
		return fmt.Errorf("set consumer prefetch: %w", err)
	}

	deliveries, err := channel.ConsumeWithContext(ctx, c.queue, "cargo-events-worker", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume queue %q: %w", c.queue, err)
	}

	if c.logger != nil {
		c.logger.InfoContext(ctx, "rabbitmq consumer started", "queue", c.queue)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq delivery channel closed")
			}
			c.handleDelivery(ctx, delivery)
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) {
	var envelope EventEnvelope
	if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
		if c.logger != nil {
			c.logger.ErrorContext(ctx, "discarding malformed event", "error", err, "message_id", delivery.MessageId)
		}
		_ = delivery.Nack(false, false) // dead-letter malformed messages
		return
	}

	if envelope.Type != CargoStatusChangedType {
		if c.logger != nil {
			c.logger.WarnContext(ctx, "discarding unsupported event type", "type", envelope.Type, "message_id", delivery.MessageId)
		}
		_ = delivery.Ack(false)
		return
	}

	event := domain.CargoStatusChanged{
		EventID:    envelope.EventID,
		CargoID:    envelope.Payload.CargoID,
		OldStatus:  envelope.Payload.OldStatus,
		NewStatus:  envelope.Payload.NewStatus,
		OccurredAt: envelope.OccurredAt,
	}

	if err := c.store.SaveCargoStatusChanged(ctx, event); err != nil {
		if c.logger != nil {
			c.logger.ErrorContext(ctx, "failed to store cargo event", "error", err, "event_id", envelope.EventID.String())
		}
		// Requeue transient failures. A production consumer should count
		// retries and move exhausted messages to the DLQ instead of hot-looping.
		_ = delivery.Nack(false, true)
		return
	}

	if c.logger != nil {
		c.logger.InfoContext(ctx, "stored cargo event",
			"event_id", envelope.EventID.String(),
			"cargo_id", envelope.Payload.CargoID.String(),
			"old_status", envelope.Payload.OldStatus,
			"new_status", envelope.Payload.NewStatus,
		)
	}

	if err := delivery.Ack(false); err != nil && c.logger != nil {
		c.logger.ErrorContext(ctx, "failed to acknowledge event", "error", err, "event_id", envelope.EventID.String())
	}
}
