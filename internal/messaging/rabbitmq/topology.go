package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	deadLetterExchange = "cargo.events.dlx"
	deadLetterQueue    = "cargo.events.dlq"
)

// declareTopology is idempotent and is called by both the producer and the
// consumer so either process can be started first.
func declareTopology(ch *amqp.Channel, exchange, queue, routingKey string) error {
	if err := ch.ExchangeDeclare(exchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %q: %w", exchange, err)
	}

	if err := ch.ExchangeDeclare(deadLetterExchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter exchange: %w", err)
	}

	queueArgs := amqp.Table{
		"x-dead-letter-exchange":    deadLetterExchange,
		"x-dead-letter-routing-key": routingKey,
	}
	if _, err := ch.QueueDeclare(queue, true, false, false, false, queueArgs); err != nil {
		return fmt.Errorf("declare queue %q: %w", queue, err)
	}
	if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
		return fmt.Errorf("bind queue %q: %w", queue, err)
	}

	if _, err := ch.QueueDeclare(deadLetterQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter queue: %w", err)
	}
	if err := ch.QueueBind(deadLetterQueue, "#", deadLetterExchange, false, nil); err != nil {
		return fmt.Errorf("bind dead-letter queue: %w", err)
	}

	return nil
}
