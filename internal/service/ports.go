package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

// CargoRepository is the persistence contract required by CargoService.
type CargoRepository interface {
	Create(ctx context.Context, cargo *domain.Cargo) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Cargo, error)
	List(ctx context.Context, filter domain.CargoFilter) ([]domain.Cargo, int, error)
	ChangeStatus(ctx context.Context, id uuid.UUID, from, to domain.CargoStatus, tracking *domain.TrackingEntry) (*domain.Cargo, error)
}

// VesselRepository is the persistence contract required by VesselService.
type VesselRepository interface {
	Create(ctx context.Context, vessel *domain.Vessel) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Vessel, error)
	List(ctx context.Context) ([]domain.Vessel, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

// TrackingRepository is the persistence contract required by TrackingService.
type TrackingRepository interface {
	Create(ctx context.Context, entry *domain.TrackingEntry) error
	ListByCargoID(ctx context.Context, cargoID uuid.UUID) ([]domain.TrackingEntry, error)
}

// EventPublisher publishes cargo lifecycle events.
type EventPublisher interface {
	PublishCargoStatusChanged(ctx context.Context, event domain.CargoStatusChanged) error
}
