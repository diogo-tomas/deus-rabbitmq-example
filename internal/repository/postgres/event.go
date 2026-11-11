package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

type EventRepository struct {
	pool *pgxpool.Pool
}

func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

// SaveCargoStatusChanged persists a consumed event. The unique event_id plus
// ON CONFLICT makes the consumer safe under RabbitMQ's at-least-once delivery.
func (r *EventRepository) SaveCargoStatusChanged(ctx context.Context, event domain.CargoStatusChanged) error {
	const query = `
		INSERT INTO cargo_events (id, event_id, cargo_id, old_status, new_status, occurred_at, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (event_id) DO NOTHING`

	_, err := r.pool.Exec(ctx, query,
		pgUUID(uuid.New()),
		pgUUID(event.EventID),
		pgUUID(event.CargoID),
		string(event.OldStatus),
		string(event.NewStatus),
		event.OccurredAt,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("save cargo event: %w", err)
	}

	return nil
}
