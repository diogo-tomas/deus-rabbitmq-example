package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/deus-rabbitmq-example/internal/domain"
	"github.com/example/deus-rabbitmq-example/internal/service"
)

func TestTrackingServiceList(t *testing.T) {
	cargoID := uuid.New()
	cargoRepo := &fakeCargoRepository{cargo: &domain.Cargo{ID: cargoID, Status: domain.CargoStatusPending}}
	trackingRepo := &fakeTrackingRepository{entries: []domain.TrackingEntry{
		{ID: uuid.New(), CargoID: cargoID, Location: "Rotterdam", Status: domain.CargoStatusPending},
	}}
	svc := service.NewTrackingService(trackingRepo, cargoRepo)

	entries, err := svc.List(context.Background(), cargoID)

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Rotterdam", entries[0].Location)
}

func TestTrackingServiceListMissingCargo(t *testing.T) {
	cargoRepo := &fakeCargoRepository{getErr: domain.ErrNotFound}
	svc := service.NewTrackingService(&fakeTrackingRepository{}, cargoRepo)

	_, err := svc.List(context.Background(), uuid.New())

	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestTrackingServiceAdd(t *testing.T) {
	cargoID := uuid.New()
	cargoRepo := &fakeCargoRepository{cargo: &domain.Cargo{ID: cargoID, Status: domain.CargoStatusInTransit}}
	trackingRepo := &fakeTrackingRepository{}
	svc := service.NewTrackingService(trackingRepo, cargoRepo)

	entry, err := svc.Add(context.Background(), cargoID, "  Rotterdam  ", time.Time{})

	require.NoError(t, err)
	assert.Equal(t, "Rotterdam", entry.Location)
	assert.Equal(t, domain.CargoStatusInTransit, entry.Status)
	assert.False(t, entry.OccurredAt.IsZero())
	assert.Same(t, entry, trackingRepo.created)
}

func TestTrackingServiceAddPreservesOccurredAt(t *testing.T) {
	cargoID := uuid.New()
	cargoRepo := &fakeCargoRepository{cargo: &domain.Cargo{ID: cargoID, Status: domain.CargoStatusPending}}
	svc := service.NewTrackingService(&fakeTrackingRepository{}, cargoRepo)
	occurredAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.FixedZone("CET", 3600))

	entry, err := svc.Add(context.Background(), cargoID, "Hamburg", occurredAt)

	require.NoError(t, err)
	assert.Equal(t, occurredAt.UTC(), entry.OccurredAt)
}

func TestTrackingServiceAddValidation(t *testing.T) {
	svc := service.NewTrackingService(&fakeTrackingRepository{}, &fakeCargoRepository{})

	_, err := svc.Add(context.Background(), uuid.New(), "   ", time.Time{})

	require.ErrorIs(t, err, domain.ErrValidation)
}

func TestTrackingServiceAddMissingCargo(t *testing.T) {
	cargoRepo := &fakeCargoRepository{getErr: domain.ErrNotFound}
	svc := service.NewTrackingService(&fakeTrackingRepository{}, cargoRepo)

	_, err := svc.Add(context.Background(), uuid.New(), "Rotterdam", time.Time{})

	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestTrackingServiceAddRepositoryError(t *testing.T) {
	cargoID := uuid.New()
	cargoRepo := &fakeCargoRepository{cargo: &domain.Cargo{ID: cargoID, Status: domain.CargoStatusPending}}
	trackingRepo := &fakeTrackingRepository{createErr: domain.ErrConflict}
	svc := service.NewTrackingService(trackingRepo, cargoRepo)

	_, err := svc.Add(context.Background(), cargoID, "Rotterdam", time.Time{})

	require.ErrorIs(t, err, domain.ErrConflict)
}
