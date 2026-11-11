package httpapi

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/example/deus-rabbitmq-example/internal/domain"
	"github.com/example/deus-rabbitmq-example/internal/service"
)

type CargoService interface {
	Create(ctx context.Context, in service.CreateCargoInput) (*domain.Cargo, error)
	Get(ctx context.Context, id uuid.UUID) (*domain.Cargo, error)
	List(ctx context.Context, filter domain.CargoFilter) ([]domain.Cargo, int, error)
	ChangeStatus(ctx context.Context, id uuid.UUID, next domain.CargoStatus, location string) (*domain.Cargo, error)
}

type VesselService interface {
	Create(ctx context.Context, in service.CreateVesselInput) (*domain.Vessel, error)
	Get(ctx context.Context, id uuid.UUID) (*domain.Vessel, error)
	List(ctx context.Context) ([]domain.Vessel, error)
}

type TrackingService interface {
	List(ctx context.Context, cargoID uuid.UUID) ([]domain.TrackingEntry, error)
	Add(ctx context.Context, cargoID uuid.UUID, location string, occurredAt time.Time) (*domain.TrackingEntry, error)
}
