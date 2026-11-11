package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/example/deus-rabbitmq-example/internal/domain"
	"github.com/example/deus-rabbitmq-example/internal/service"
	"github.com/example/deus-rabbitmq-example/internal/transport/httpapi"
)

type fakePinger struct {
	err error
}

func (f *fakePinger) Ping(ctx context.Context) error {
	return f.err
}

func TestCargoHandlerRejectsInvalidInputs(t *testing.T) {
	vesselID := uuid.New()

	t.Run("invalid vessel id on create", func(t *testing.T) {
		body := `{"code":"C-1","description":"test","weight_kg":10,"vessel_id":"not-a-uuid"}`
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPost, "/api/v1/cargoes", body)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("invalid vessel id filter", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes?vessel_id=not-a-uuid", "")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("invalid limit", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes?limit=0", "")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("invalid offset", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes?offset=-1", "")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("limit is capped", func(t *testing.T) {
		cargoService := &fakeCargoService{
			listFn: func(ctx context.Context, filter domain.CargoFilter) ([]domain.Cargo, int, error) {
				assert.Equal(t, 100, filter.Limit)
				return nil, 0, nil
			},
		}
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes?limit=500", "")

		assert.Equal(t, http.StatusOK, recorder.Code)
	})

	t.Run("invalid cargo id on status change", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPatch, "/api/v1/cargoes/not-a-uuid/status", `{"status":"in_transit","location":"Rotterdam"}`)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("malformed status change body", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPatch, "/api/v1/cargoes/"+vesselID.String()+"/status", "{")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("status validation failure maps to 422", func(t *testing.T) {
		cargoService := &fakeCargoService{
			changeStatusFn: func(ctx context.Context, id uuid.UUID, next domain.CargoStatus, location string) (*domain.Cargo, error) {
				return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "status", Message: "is invalid"}}}
			},
		}
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPatch, "/api/v1/cargoes/"+vesselID.String()+"/status", `{"status":"lost","location":"Rotterdam"}`)

		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	})

	t.Run("internal errors are not leaked", func(t *testing.T) {
		cargoService := &fakeCargoService{
			getFn: func(ctx context.Context, id uuid.UUID) (*domain.Cargo, error) {
				return nil, errors.New("database exploded")
			},
		}
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes/"+uuid.NewString(), "")

		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "internal_error")
		assert.NotContains(t, recorder.Body.String(), "database exploded")
	})
}

func TestVesselHandlerErrors(t *testing.T) {
	t.Run("invalid id", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/vessels/not-a-uuid", "")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("not found", func(t *testing.T) {
		vesselService := &fakeVesselService{
			getFn: func(ctx context.Context, id uuid.UUID) (*domain.Vessel, error) {
				return nil, domain.ErrNotFound
			},
		}
		recorder := performRequest(testRouter(&fakeCargoService{}, vesselService, &fakeTrackingService{}), http.MethodGet, "/api/v1/vessels/"+uuid.NewString(), "")

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("validation failure", func(t *testing.T) {
		vesselService := &fakeVesselService{
			createFn: func(ctx context.Context, in service.CreateVesselInput) (*domain.Vessel, error) {
				return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "name", Message: "is required"}}}
			},
		}
		recorder := performRequest(testRouter(&fakeCargoService{}, vesselService, &fakeTrackingService{}), http.MethodPost, "/api/v1/vessels", `{"name":"","capacity":0,"current_location":""}`)

		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	})
}

func TestTrackingHandlerErrors(t *testing.T) {
	t.Run("invalid cargo id", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes/not-a-uuid/tracking", "")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("cargo not found", func(t *testing.T) {
		trackingService := &fakeTrackingService{
			listFn: func(ctx context.Context, cargoID uuid.UUID) ([]domain.TrackingEntry, error) {
				return nil, domain.ErrNotFound
			},
		}
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, trackingService), http.MethodGet, "/api/v1/cargoes/"+uuid.NewString()+"/tracking", "")

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("malformed tracking body", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPost, "/api/v1/cargoes/"+uuid.NewString()+"/tracking", "{")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestReadyz(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		router := httpapi.NewRouter(
			httpapi.NewCargoHandler(&fakeCargoService{}, testLogger()),
			httpapi.NewVesselHandler(&fakeVesselService{}, testLogger()),
			httpapi.NewTrackingHandler(&fakeTrackingService{}, testLogger()),
			&fakePinger{},
			testLogger(),
		)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		assert.Equal(t, http.StatusOK, recorder.Code)
	})

	t.Run("not ready", func(t *testing.T) {
		router := httpapi.NewRouter(
			httpapi.NewCargoHandler(&fakeCargoService{}, testLogger()),
			httpapi.NewVesselHandler(&fakeVesselService{}, testLogger()),
			httpapi.NewTrackingHandler(&fakeTrackingService{}, testLogger()),
			&fakePinger{err: errors.New("database unavailable")},
			testLogger(),
		)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	})
}
