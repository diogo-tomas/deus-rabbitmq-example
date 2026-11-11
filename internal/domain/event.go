package domain

import (
	"time"

	"github.com/google/uuid"
)

// CargoStatusChanged is emitted after a successful status transition.
type CargoStatusChanged struct {
	EventID    uuid.UUID
	CargoID    uuid.UUID
	OldStatus  CargoStatus
	NewStatus  CargoStatus
	OccurredAt time.Time
}
