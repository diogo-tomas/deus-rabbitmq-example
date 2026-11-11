//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/deus-rabbitmq-example/internal/domain"
	"github.com/example/deus-rabbitmq-example/internal/platform/db"
	"github.com/example/deus-rabbitmq-example/internal/repository/postgres"
	"github.com/example/deus-rabbitmq-example/migrations"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	require.NoError(t, migrations.Up(dsn))

	pool, err := db.NewPool(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	_, err = pool.Exec(context.Background(), `TRUNCATE TABLE cargo_events, tracking, cargoes, vessels RESTART IDENTITY CASCADE`)
	require.NoError(t, err)

	return pool
}

func seedVessel(t *testing.T, repo *postgres.VesselRepository) *domain.Vessel {
	t.Helper()

	now := time.Now().UTC()
	vessel := &domain.Vessel{
		ID:              uuid.New(),
		Name:            "MV Test " + uuid.NewString(),
		Capacity:        100,
		CurrentLocation: "Rotterdam",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, repo.Create(context.Background(), vessel))

	return vessel
}

func seedCargo(t *testing.T, repo *postgres.CargoRepository, vesselID uuid.UUID) *domain.Cargo {
	t.Helper()

	now := time.Now().UTC()
	cargo := &domain.Cargo{
		ID:          uuid.New(),
		Code:        "C-" + uuid.NewString(),
		Description: "test cargo",
		WeightKg:    10.5,
		Status:      domain.CargoStatusPending,
		VesselID:    vesselID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, repo.Create(context.Background(), cargo))

	return cargo
}

func changeStatus(t *testing.T, repo *postgres.CargoRepository, cargo *domain.Cargo, to domain.CargoStatus) *domain.Cargo {
	t.Helper()

	now := time.Now().UTC()
	entry := &domain.TrackingEntry{
		ID:         uuid.New(),
		CargoID:    cargo.ID,
		Location:   "Rotterdam",
		Status:     to,
		OccurredAt: now,
		CreatedAt:  now,
	}

	updated, err := repo.ChangeStatus(context.Background(), cargo.ID, cargo.Status, to, entry)
	require.NoError(t, err)

	return updated
}

func TestCargoRepositoryCreateGetAndConflict(t *testing.T) {
	pool := newTestPool(t)
	vesselRepo := postgres.NewVesselRepository(pool)
	cargoRepo := postgres.NewCargoRepository(pool)

	vessel := seedVessel(t, vesselRepo)
	cargo := seedCargo(t, cargoRepo, vessel.ID)

	got, err := cargoRepo.GetByID(context.Background(), cargo.ID)
	require.NoError(t, err)
	assert.Equal(t, cargo.Code, got.Code)
	assert.Equal(t, cargo.Description, got.Description)
	assert.Equal(t, cargo.WeightKg, got.WeightKg)
	assert.Equal(t, domain.CargoStatusPending, got.Status)
	assert.Equal(t, vessel.ID, got.VesselID)

	duplicate := *cargo
	duplicate.ID = uuid.New()
	require.ErrorIs(t, cargoRepo.Create(context.Background(), &duplicate), domain.ErrConflict)

	_, err = cargoRepo.GetByID(context.Background(), uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCargoRepositoryListFiltersAndPagination(t *testing.T) {
	pool := newTestPool(t)
	vesselRepo := postgres.NewVesselRepository(pool)
	cargoRepo := postgres.NewCargoRepository(pool)

	vessel := seedVessel(t, vesselRepo)
	pending := seedCargo(t, cargoRepo, vessel.ID)
	inTransit := seedCargo(t, cargoRepo, vessel.ID)
	delivered := seedCargo(t, cargoRepo, vessel.ID)

	inTransit = changeStatus(t, cargoRepo, inTransit, domain.CargoStatusInTransit)
	delivered = changeStatus(t, cargoRepo, delivered, domain.CargoStatusInTransit)
	changeStatus(t, cargoRepo, delivered, domain.CargoStatusDelivered)

	status := domain.CargoStatusPending
	items, total, err := cargoRepo.List(context.Background(), domain.CargoFilter{Status: &status, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, pending.ID, items[0].ID)

	items, total, err = cargoRepo.List(context.Background(), domain.CargoFilter{VesselID: &vessel.ID, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, items, 3)

	items, total, err = cargoRepo.List(context.Background(), domain.CargoFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, items, 2)

	items, total, err = cargoRepo.List(context.Background(), domain.CargoFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, items, 1)
}

func TestCargoRepositoryChangeStatusAppendsTracking(t *testing.T) {
	pool := newTestPool(t)
	vesselRepo := postgres.NewVesselRepository(pool)
	cargoRepo := postgres.NewCargoRepository(pool)
	trackingRepo := postgres.NewTrackingRepository(pool)

	vessel := seedVessel(t, vesselRepo)
	cargo := seedCargo(t, cargoRepo, vessel.ID)

	inTransit := changeStatus(t, cargoRepo, cargo, domain.CargoStatusInTransit)
	assert.Equal(t, domain.CargoStatusInTransit, inTransit.Status)

	entries, err := trackingRepo.ListByCargoID(context.Background(), cargo.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Rotterdam", entries[0].Location)
	assert.Equal(t, domain.CargoStatusInTransit, entries[0].Status)

	// A concurrent request that still believes the cargo is pending must fail.
	now := time.Now().UTC()
	staleEntry := &domain.TrackingEntry{
		ID:         uuid.New(),
		CargoID:    cargo.ID,
		Location:   "Hamburg",
		Status:     domain.CargoStatusInTransit,
		OccurredAt: now,
		CreatedAt:  now,
	}
	_, err = cargoRepo.ChangeStatus(context.Background(), cargo.ID, domain.CargoStatusPending, domain.CargoStatusInTransit, staleEntry)
	require.ErrorIs(t, err, domain.ErrConflict)

	delivered := changeStatus(t, cargoRepo, inTransit, domain.CargoStatusDelivered)
	assert.Equal(t, domain.CargoStatusDelivered, delivered.Status)

	entries, err = trackingRepo.ListByCargoID(context.Background(), cargo.ID)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestEventRepositoryIsIdempotent(t *testing.T) {
	pool := newTestPool(t)
	vesselRepo := postgres.NewVesselRepository(pool)
	cargoRepo := postgres.NewCargoRepository(pool)
	eventRepo := postgres.NewEventRepository(pool)

	vessel := seedVessel(t, vesselRepo)
	cargo := seedCargo(t, cargoRepo, vessel.ID)

	event := domain.CargoStatusChanged{
		EventID:    uuid.New(),
		CargoID:    cargo.ID,
		OldStatus:  domain.CargoStatusPending,
		NewStatus:  domain.CargoStatusInTransit,
		OccurredAt: time.Now().UTC(),
	}

	require.NoError(t, eventRepo.SaveCargoStatusChanged(context.Background(), event))
	require.NoError(t, eventRepo.SaveCargoStatusChanged(context.Background(), event))

	var count int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM cargo_events`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestVesselRepository(t *testing.T) {
	pool := newTestPool(t)
	vesselRepo := postgres.NewVesselRepository(pool)

	vessel := seedVessel(t, vesselRepo)

	got, err := vesselRepo.GetByID(context.Background(), vessel.ID)
	require.NoError(t, err)
	assert.Equal(t, vessel.Name, got.Name)
	assert.Equal(t, vessel.Capacity, got.Capacity)

	exists, err := vesselRepo.Exists(context.Background(), vessel.ID)
	require.NoError(t, err)
	assert.True(t, exists)

	vessels, err := vesselRepo.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, vessels, 1)

	duplicate := *vessel
	duplicate.ID = uuid.New()
	require.ErrorIs(t, vesselRepo.Create(context.Background(), &duplicate), domain.ErrConflict)
}
