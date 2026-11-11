package domain

import (
	"time"

	"github.com/google/uuid"
)

// CargoStatus is the lifecycle state of a cargo.
type CargoStatus string

const (
	CargoStatusPending   CargoStatus = "pending"
	CargoStatusInTransit CargoStatus = "in_transit"
	CargoStatusDelivered CargoStatus = "delivered"
)

// Valid reports whether s is a supported cargo status.
func (s CargoStatus) Valid() bool {
	switch s {
	case CargoStatusPending, CargoStatusInTransit, CargoStatusDelivered:
		return true
	default:
		return false
	}
}

// CanTransitionTo encodes the allowed lifecycle:
// pending -> in_transit -> delivered. Every other transition is rejected.
func (s CargoStatus) CanTransitionTo(next CargoStatus) bool {
	switch s {
	case CargoStatusPending:
		return next == CargoStatusInTransit
	case CargoStatusInTransit:
		return next == CargoStatusDelivered
	default:
		return false
	}
}

// Cargo is a shipment moving through the logistics network.
type Cargo struct {
	ID          uuid.UUID
	Code        string
	Description string
	WeightKg    float64
	Status      CargoStatus
	VesselID    uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CargoFilter holds the supported filters for listing cargoes.
type CargoFilter struct {
	Status   *CargoStatus
	VesselID *uuid.UUID
	Limit    int
	Offset   int
}
