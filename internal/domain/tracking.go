package domain

import (
	"time"

	"github.com/google/uuid"
)

// TrackingEntry is an append-only record of a cargo movement.
type TrackingEntry struct {
	ID         uuid.UUID
	CargoID    uuid.UUID
	Location   string
	Status     CargoStatus
	OccurredAt time.Time
	CreatedAt  time.Time
}
