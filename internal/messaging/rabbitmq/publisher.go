package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

// Publisher publishes cargo lifecycle events using publisher confirms.
type Publisher struct {
	conn       *amqp.Connection
	channel    *amqp.Channel
	exchange   string
	routingKey string
	logger     *slog.Logger

	mu sync.Mutex
}

func NewPublisher(url, exchange, queue, routingKey string, logger *slog.Logger) (*Publisher, error) {
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(10 * time.Second)})
	if err != nil {
		return nil, fmt.Errorf("connect to rabbitmq: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}

	if err := declareTopology(channel, exchange, queue, routingKey); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}

	return &Publisher{
		conn:       conn,
		channel:    channel,
		exchange:   exchange,
		routingKey: routingKey,
		logger:     logger,
	}, nil
}

func (p *Publisher) PublishCargoStatusChanged(ctx context.Context, event domain.CargoStatusChanged) error {
	body, err := json.Marshal(newCargoStatusChangedEnvelope(event))
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	message := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    event.EventID.String(),
		Timestamp:    event.OccurredAt,
		Type:         CargoStatusChangedType,
		Body:         body,
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, p.exchange, p.routingKey, false, false, message)
	if err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	if confirmation == nil {
		return fmt.Errorf("publisher confirms are not enabled")
	}

	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for rabbitmq confirmation: %w", err)
	}
	if !acked {
		return fmt.Errorf("rabbitmq rejected event %s", event.EventID)
	}

	if p.logger != nil {
		p.logger.DebugContext(ctx, "published cargo status changed event",
			"event_id", event.EventID.String(),
			"cargo_id", event.CargoID.String(),
		)
	}

	return nil
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}

	return nil
}
