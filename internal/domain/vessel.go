package domain

import (
	"time"

	"github.com/google/uuid"
)

// Vessel is a ship that transports cargoes.
type Vessel struct {
	ID              uuid.UUID
	Name            string
	Capacity        int
	CurrentLocation string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
