package rabbitmq

import (
	"time"

	"github.com/google/uuid"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

const CargoStatusChangedType = "cargo.status_changed"

// EventEnvelope is the wire format for all cargo events.
type EventEnvelope struct {
	EventID    uuid.UUID          `json:"event_id"`
	Type       string             `json:"type"`
	OccurredAt time.Time          `json:"occurred_at"`
	Payload    CargoStatusPayload `json:"payload"`
}

// CargoStatusPayload is the payload of a cargo.status_changed event.
type CargoStatusPayload struct {
	CargoID   uuid.UUID          `json:"cargo_id"`
	OldStatus domain.CargoStatus `json:"old_status"`
	NewStatus domain.CargoStatus `json:"new_status"`
}

func newCargoStatusChangedEnvelope(event domain.CargoStatusChanged) EventEnvelope {
	return EventEnvelope{
		EventID:    event.EventID,
		Type:       CargoStatusChangedType,
		OccurredAt: event.OccurredAt,
		Payload: CargoStatusPayload{
			CargoID:   event.CargoID,
			OldStatus: event.OldStatus,
			NewStatus: event.NewStatus,
		},
	}
}
