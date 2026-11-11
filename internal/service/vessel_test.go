package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/deus-rabbitmq-example/internal/domain"
	"github.com/example/deus-rabbitmq-example/internal/service"
)

func TestVesselServiceCreate(t *testing.T) {
	t.Run("creates a vessel", func(t *testing.T) {
		repo := &fakeVesselRepository{}
		svc := service.NewVesselService(repo)

		vessel, err := svc.Create(context.Background(), service.CreateVesselInput{
			Name:            "  MV Test  ",
			Capacity:        100,
			CurrentLocation: "  Rotterdam  ",
		})

		require.NoError(t, err)
		assert.Equal(t, "MV Test", vessel.Name)
		assert.Equal(t, "Rotterdam", vessel.CurrentLocation)
		assert.NotEqual(t, uuid.Nil, vessel.ID)
		assert.Same(t, vessel, repo.created)
	})

	t.Run("validates input", func(t *testing.T) {
		repo := &fakeVesselRepository{}
		svc := service.NewVesselService(repo)

		_, err := svc.Create(context.Background(), service.CreateVesselInput{})

		require.ErrorIs(t, err, domain.ErrValidation)
		var validation *domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Len(t, validation.Fields, 3)
	})

	t.Run("propagates conflicts", func(t *testing.T) {
		repo := &fakeVesselRepository{createErr: domain.ErrConflict}
		svc := service.NewVesselService(repo)

		_, err := svc.Create(context.Background(), service.CreateVesselInput{
			Name:            "MV Test",
			Capacity:        100,
			CurrentLocation: "Rotterdam",
		})

		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestVesselServiceGet(t *testing.T) {
	id := uuid.New()
	repo := &fakeVesselRepository{vessel: &domain.Vessel{ID: id, Name: "MV Test"}}
	svc := service.NewVesselService(repo)

	vessel, err := svc.Get(context.Background(), id)

	require.NoError(t, err)
	assert.Equal(t, id, vessel.ID)
}

func TestVesselServiceGetNotFound(t *testing.T) {
	repo := &fakeVesselRepository{getErr: domain.ErrNotFound}
	svc := service.NewVesselService(repo)

	_, err := svc.Get(context.Background(), uuid.New())

	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestVesselServiceList(t *testing.T) {
	repo := &fakeVesselRepository{listErr: domain.ErrConflict}
	svc := service.NewVesselService(repo)

	_, err := svc.List(context.Background())

	require.ErrorIs(t, err, domain.ErrConflict)
}
