package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/deus-rabbitmq-example/internal/domain"
	"github.com/example/deus-rabbitmq-example/internal/service"
)

type fakeCargoRepository struct {
	cargo     *domain.Cargo
	changed   *domain.Cargo
	created   *domain.Cargo
	createErr error
	getErr    error
	listErr   error
	changeErr error

	lastFilter   domain.CargoFilter
	lastTracking *domain.TrackingEntry
	lastFrom     domain.CargoStatus
	lastTo       domain.CargoStatus
}

func (f *fakeCargoRepository) Create(ctx context.Context, cargo *domain.Cargo) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = cargo
	return nil
}

func (f *fakeCargoRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Cargo, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.cargo == nil {
		return nil, domain.ErrNotFound
	}
	return f.cargo, nil
}

func (f *fakeCargoRepository) List(ctx context.Context, filter domain.CargoFilter) ([]domain.Cargo, int, error) {
	f.lastFilter = filter
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return nil, 0, nil
}

func (f *fakeCargoRepository) ChangeStatus(ctx context.Context, id uuid.UUID, from, to domain.CargoStatus, tracking *domain.TrackingEntry) (*domain.Cargo, error) {
	f.lastFrom = from
	f.lastTo = to
	f.lastTracking = tracking
	if f.changeErr != nil {
		return nil, f.changeErr
	}
	if f.changed != nil {
		return f.changed, nil
	}

	updated := *f.cargo
	updated.Status = to
	return &updated, nil
}

type fakeVesselRepository struct {
	vessel    *domain.Vessel
	created   *domain.Vessel
	exists    bool
	createErr error
	getErr    error
	listErr   error
	existsErr error
}

func (f *fakeVesselRepository) Create(ctx context.Context, vessel *domain.Vessel) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = vessel
	return nil
}

func (f *fakeVesselRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Vessel, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.vessel == nil {
		return nil, domain.ErrNotFound
	}
	return f.vessel, nil
}

func (f *fakeVesselRepository) List(ctx context.Context) ([]domain.Vessel, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return nil, nil
}

func (f *fakeVesselRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	return f.exists, f.existsErr
}

type fakeTrackingRepository struct {
	created   *domain.TrackingEntry
	entries   []domain.TrackingEntry
	createErr error
	listErr   error
}

func (f *fakeTrackingRepository) Create(ctx context.Context, entry *domain.TrackingEntry) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = entry
	return nil
}

func (f *fakeTrackingRepository) ListByCargoID(ctx context.Context, cargoID uuid.UUID) ([]domain.TrackingEntry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.entries, nil
}

type fakePublisher struct {
	events []domain.CargoStatusChanged
	err    error
}

func (f *fakePublisher) PublishCargoStatusChanged(ctx context.Context, event domain.CargoStatusChanged) error {
	f.events = append(f.events, event)
	return f.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCargoServiceCreate(t *testing.T) {
	vesselID := uuid.New()

	t.Run("creates a pending cargo", func(t *testing.T) {
		repo := &fakeCargoRepository{}
		vessels := &fakeVesselRepository{exists: true}
		svc := service.NewCargoService(repo, vessels, &fakePublisher{}, testLogger())

		cargo, err := svc.Create(context.Background(), service.CreateCargoInput{
			Code:        "  C-1  ",
			Description: "  fragile  ",
			WeightKg:    12.5,
			VesselID:    vesselID,
		})

		require.NoError(t, err)
		require.NotNil(t, cargo)
		assert.Equal(t, "C-1", cargo.Code)
		assert.Equal(t, "fragile", cargo.Description)
		assert.Equal(t, domain.CargoStatusPending, cargo.Status)
		assert.NotEqual(t, uuid.Nil, cargo.ID)
		assert.Same(t, cargo, repo.created)
	})

	t.Run("rejects invalid input before touching the repository", func(t *testing.T) {
		repo := &fakeCargoRepository{}
		vessels := &fakeVesselRepository{exists: true}
		svc := service.NewCargoService(repo, vessels, &fakePublisher{}, testLogger())

		_, err := svc.Create(context.Background(), service.CreateCargoInput{})

		require.Error(t, err)
		require.ErrorIs(t, err, domain.ErrValidation)

		var validation *domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Len(t, validation.Fields, 3)
		assert.Nil(t, repo.created)
	})

	t.Run("returns not found when the vessel does not exist", func(t *testing.T) {
		repo := &fakeCargoRepository{}
		vessels := &fakeVesselRepository{exists: false}
		svc := service.NewCargoService(repo, vessels, &fakePublisher{}, testLogger())

		_, err := svc.Create(context.Background(), service.CreateCargoInput{
			Code:     "C-1",
			WeightKg: 1,
			VesselID: vesselID,
		})

		require.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, repo.created)
	})

	t.Run("wraps vessel lookup failures", func(t *testing.T) {
		repo := &fakeCargoRepository{}
		vessels := &fakeVesselRepository{existsErr: errors.New("db down")}
		svc := service.NewCargoService(repo, vessels, &fakePublisher{}, testLogger())

		_, err := svc.Create(context.Background(), service.CreateCargoInput{
			Code:     "C-1",
			WeightKg: 1,
			VesselID: vesselID,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "check vessel")
	})

	t.Run("propagates duplicate cargo conflicts", func(t *testing.T) {
		repo := &fakeCargoRepository{createErr: domain.ErrConflict}
		vessels := &fakeVesselRepository{exists: true}
		svc := service.NewCargoService(repo, vessels, &fakePublisher{}, testLogger())

		_, err := svc.Create(context.Background(), service.CreateCargoInput{
			Code:     "C-1",
			WeightKg: 1,
			VesselID: vesselID,
		})

		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestCargoServiceGet(t *testing.T) {
	t.Run("returns the cargo", func(t *testing.T) {
		id := uuid.New()
		repo := &fakeCargoRepository{cargo: &domain.Cargo{ID: id, Code: "C-1"}}
		svc := service.NewCargoService(repo, &fakeVesselRepository{}, &fakePublisher{}, testLogger())

		cargo, err := svc.Get(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, id, cargo.ID)
	})

	t.Run("propagates not found", func(t *testing.T) {
		repo := &fakeCargoRepository{getErr: domain.ErrNotFound}
		svc := service.NewCargoService(repo, &fakeVesselRepository{}, &fakePublisher{}, testLogger())

		_, err := svc.Get(context.Background(), uuid.New())

		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestCargoServiceList(t *testing.T) {
	t.Run("applies defaults and caps", func(t *testing.T) {
		repo := &fakeCargoRepository{}
		svc := service.NewCargoService(repo, &fakeVesselRepository{}, &fakePublisher{}, testLogger())

		_, _, err := svc.List(context.Background(), domain.CargoFilter{})
		require.NoError(t, err)
		assert.Equal(t, 20, repo.lastFilter.Limit)
		assert.Equal(t, 0, repo.lastFilter.Offset)

		_, _, err = svc.List(context.Background(), domain.CargoFilter{Limit: 500, Offset: -3})
		require.NoError(t, err)
		assert.Equal(t, 100, repo.lastFilter.Limit)
		assert.Equal(t, 0, repo.lastFilter.Offset)
	})

	t.Run("rejects an invalid status filter", func(t *testing.T) {
		repo := &fakeCargoRepository{}
		svc := service.NewCargoService(repo, &fakeVesselRepository{}, &fakePublisher{}, testLogger())
		status := domain.CargoStatus("lost")

		_, _, err := svc.List(context.Background(), domain.CargoFilter{Status: &status})

		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestCargoServiceChangeStatus(t *testing.T) {
	cargoID := uuid.New()

	t.Run("updates cargo, appends tracking, and publishes an event", func(t *testing.T) {
		repo := &fakeCargoRepository{
			cargo:   &domain.Cargo{ID: cargoID, Status: domain.CargoStatusPending},
			changed: &domain.Cargo{ID: cargoID, Status: domain.CargoStatusInTransit},
		}
		publisher := &fakePublisher{}
		svc := service.NewCargoService(repo, &fakeVesselRepository{}, publisher, testLogger())

		cargo, err := svc.ChangeStatus(context.Background(), cargoID, domain.CargoStatusInTransit, " Rotterdam ")

		require.NoError(t, err)
		assert.Equal(t, domain.CargoStatusInTransit, cargo.Status)
		assert.Equal(t, domain.CargoStatusPending, repo.lastFrom)
		assert.Equal(t, domain.CargoStatusInTransit, repo.lastTo)
		require.NotNil(t, repo.lastTracking)
		assert.Equal(t, "Rotterdam", repo.lastTracking.Location)
		assert.Equal(t, domain.CargoStatusInTransit, repo.lastTracking.Status)
		require.Len(t, publisher.events, 1)
		assert.Equal(t, domain.CargoStatusPending, publisher.events[0].OldStatus)
		assert.Equal(t, domain.CargoStatusInTransit, publisher.events[0].NewStatus)
	})

	t.Run("rejects an invalid transition", func(t *testing.T) {
		repo := &fakeCargoRepository{
			cargo: &domain.Cargo{ID: cargoID, Status: domain.CargoStatusInTransit},
		}
		publisher := &fakePublisher{}
		svc := service.NewCargoService(repo, &fakeVesselRepository{}, publisher, testLogger())

		_, err := svc.ChangeStatus(context.Background(), cargoID, domain.CargoStatusPending, "Rotterdam")

		require.ErrorIs(t, err, domain.ErrInvalidTransition)
		assert.Nil(t, repo.lastTracking)
		assert.Empty(t, publisher.events)
	})

	t.Run("validates status and location", func(t *testing.T) {
		repo := &fakeCargoRepository{}
		svc := service.NewCargoService(repo, &fakeVesselRepository{}, &fakePublisher{}, testLogger())

		_, err := svc.ChangeStatus(context.Background(), cargoID, domain.CargoStatus("lost"), "")

		require.ErrorIs(t, err, domain.ErrValidation)
		var validation *domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Len(t, validation.Fields, 2)
	})

	t.Run("propagates stale status conflicts", func(t *testing.T) {
		repo := &fakeCargoRepository{
			cargo:     &domain.Cargo{ID: cargoID, Status: domain.CargoStatusPending},
			changeErr: domain.ErrConflict,
		}
		svc := service.NewCargoService(repo, &fakeVesselRepository{}, &fakePublisher{}, testLogger())

		_, err := svc.ChangeStatus(context.Background(), cargoID, domain.CargoStatusInTransit, "Rotterdam")

		require.ErrorIs(t, err, domain.ErrConflict)
	})

	t.Run("does not fail the request when publishing fails", func(t *testing.T) {
		repo := &fakeCargoRepository{
			cargo:   &domain.Cargo{ID: cargoID, Status: domain.CargoStatusPending},
			changed: &domain.Cargo{ID: cargoID, Status: domain.CargoStatusInTransit},
		}
		publisher := &fakePublisher{err: errors.New("broker down")}
		svc := service.NewCargoService(repo, &fakeVesselRepository{}, publisher, testLogger())

		cargo, err := svc.ChangeStatus(context.Background(), cargoID, domain.CargoStatusInTransit, "Rotterdam")

		require.NoError(t, err)
		assert.Equal(t, domain.CargoStatusInTransit, cargo.Status)
		assert.Len(t, publisher.events, 1)
	})
}
