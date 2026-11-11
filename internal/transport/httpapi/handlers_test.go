package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/deus-rabbitmq-example/internal/domain"
	"github.com/example/deus-rabbitmq-example/internal/service"
	"github.com/example/deus-rabbitmq-example/internal/transport/httpapi"
)

type fakeCargoService struct {
	createFn       func(context.Context, service.CreateCargoInput) (*domain.Cargo, error)
	getFn          func(context.Context, uuid.UUID) (*domain.Cargo, error)
	listFn         func(context.Context, domain.CargoFilter) ([]domain.Cargo, int, error)
	changeStatusFn func(context.Context, uuid.UUID, domain.CargoStatus, string) (*domain.Cargo, error)
}

func (f *fakeCargoService) Create(ctx context.Context, in service.CreateCargoInput) (*domain.Cargo, error) {
	if f.createFn == nil {
		return nil, errors.New("unexpected Create call")
	}
	return f.createFn(ctx, in)
}

func (f *fakeCargoService) Get(ctx context.Context, id uuid.UUID) (*domain.Cargo, error) {
	if f.getFn == nil {
		return nil, errors.New("unexpected Get call")
	}
	return f.getFn(ctx, id)
}

func (f *fakeCargoService) List(ctx context.Context, filter domain.CargoFilter) ([]domain.Cargo, int, error) {
	if f.listFn == nil {
		return nil, 0, errors.New("unexpected List call")
	}
	return f.listFn(ctx, filter)
}

func (f *fakeCargoService) ChangeStatus(ctx context.Context, id uuid.UUID, next domain.CargoStatus, location string) (*domain.Cargo, error) {
	if f.changeStatusFn == nil {
		return nil, errors.New("unexpected ChangeStatus call")
	}
	return f.changeStatusFn(ctx, id, next, location)
}

type fakeVesselService struct {
	createFn func(context.Context, service.CreateVesselInput) (*domain.Vessel, error)
	getFn    func(context.Context, uuid.UUID) (*domain.Vessel, error)
	listFn   func(context.Context) ([]domain.Vessel, error)
}

func (f *fakeVesselService) Create(ctx context.Context, in service.CreateVesselInput) (*domain.Vessel, error) {
	if f.createFn == nil {
		return nil, errors.New("unexpected Create call")
	}
	return f.createFn(ctx, in)
}

func (f *fakeVesselService) Get(ctx context.Context, id uuid.UUID) (*domain.Vessel, error) {
	if f.getFn == nil {
		return nil, errors.New("unexpected Get call")
	}
	return f.getFn(ctx, id)
}

func (f *fakeVesselService) List(ctx context.Context) ([]domain.Vessel, error) {
	if f.listFn == nil {
		return nil, errors.New("unexpected List call")
	}
	return f.listFn(ctx)
}

type fakeTrackingService struct {
	listFn func(context.Context, uuid.UUID) ([]domain.TrackingEntry, error)
	addFn  func(context.Context, uuid.UUID, string, time.Time) (*domain.TrackingEntry, error)
}

func (f *fakeTrackingService) List(ctx context.Context, cargoID uuid.UUID) ([]domain.TrackingEntry, error) {
	if f.listFn == nil {
		return nil, errors.New("unexpected List call")
	}
	return f.listFn(ctx, cargoID)
}

func (f *fakeTrackingService) Add(ctx context.Context, cargoID uuid.UUID, location string, occurredAt time.Time) (*domain.TrackingEntry, error) {
	if f.addFn == nil {
		return nil, errors.New("unexpected Add call")
	}
	return f.addFn(ctx, cargoID, location, occurredAt)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testRouter(cargo *fakeCargoService, vessel *fakeVesselService, tracking *fakeTrackingService) http.Handler {
	return httpapi.NewRouter(
		httpapi.NewCargoHandler(cargo, testLogger()),
		httpapi.NewVesselHandler(vessel, testLogger()),
		httpapi.NewTrackingHandler(tracking, testLogger()),
		nil,
		testLogger(),
	)
}

func performRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	request := httptest.NewRequest(method, path, reader)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func TestCargoHandlerCreate(t *testing.T) {
	vesselID := uuid.New()
	now := time.Now().UTC()

	t.Run("creates a cargo", func(t *testing.T) {
		cargoService := &fakeCargoService{
			createFn: func(ctx context.Context, in service.CreateCargoInput) (*domain.Cargo, error) {
				assert.Equal(t, "C-1", in.Code)
				assert.Equal(t, vesselID, in.VesselID)
				return &domain.Cargo{
					ID:          uuid.New(),
					Code:        in.Code,
					Description: in.Description,
					WeightKg:    in.WeightKg,
					Status:      domain.CargoStatusPending,
					VesselID:    in.VesselID,
					CreatedAt:   now,
					UpdatedAt:   now,
				}, nil
			},
		}

		body := `{"code":"C-1","description":"test","weight_kg":10,"vessel_id":"` + vesselID.String() + `"}`
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPost, "/api/v1/cargoes", body)

		require.Equal(t, http.StatusCreated, recorder.Code)
		assert.NotEmpty(t, recorder.Header().Get("Location"))
		assert.NotEmpty(t, recorder.Header().Get("X-Request-ID"))

		var response map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Equal(t, "C-1", response["code"])
		assert.Equal(t, "pending", response["status"])
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPost, "/api/v1/cargoes", "{")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "bad_request")
	})

	t.Run("returns validation errors", func(t *testing.T) {
		cargoService := &fakeCargoService{
			createFn: func(ctx context.Context, in service.CreateCargoInput) (*domain.Cargo, error) {
				return nil, &domain.ValidationError{Fields: []domain.FieldError{
					{Field: "code", Message: "is required"},
				}}
			},
		}
		body := `{"code":"","description":"test","weight_kg":10,"vessel_id":"` + vesselID.String() + `"}`
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPost, "/api/v1/cargoes", body)

		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "validation_error")
		assert.Contains(t, recorder.Body.String(), "code")
	})

	t.Run("returns not found for a missing vessel", func(t *testing.T) {
		cargoService := &fakeCargoService{
			createFn: func(ctx context.Context, in service.CreateCargoInput) (*domain.Cargo, error) {
				return nil, fmt.Errorf("%w: vessel %s", domain.ErrNotFound, in.VesselID)
			},
		}
		body := `{"code":"C-1","description":"test","weight_kg":10,"vessel_id":"` + vesselID.String() + `"}`
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPost, "/api/v1/cargoes", body)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})
}

func TestCargoHandlerGet(t *testing.T) {
	cargoID := uuid.New()

	t.Run("returns a cargo", func(t *testing.T) {
		cargoService := &fakeCargoService{
			getFn: func(ctx context.Context, id uuid.UUID) (*domain.Cargo, error) {
				assert.Equal(t, cargoID, id)
				return &domain.Cargo{ID: id, Code: "C-1", Status: domain.CargoStatusPending}, nil
			},
		}
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes/"+cargoID.String(), "")

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "C-1")
	})

	t.Run("rejects an invalid id", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes/not-a-uuid", "")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("returns not found", func(t *testing.T) {
		cargoService := &fakeCargoService{
			getFn: func(ctx context.Context, id uuid.UUID) (*domain.Cargo, error) {
				return nil, domain.ErrNotFound
			},
		}
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes/"+cargoID.String(), "")

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})
}

func TestCargoHandlerList(t *testing.T) {
	t.Run("returns a paginated list", func(t *testing.T) {
		cargoService := &fakeCargoService{
			listFn: func(ctx context.Context, filter domain.CargoFilter) ([]domain.Cargo, int, error) {
				require.NotNil(t, filter.Status)
				assert.Equal(t, domain.CargoStatusPending, *filter.Status)
				assert.Equal(t, 5, filter.Limit)
				assert.Equal(t, 2, filter.Offset)
				return []domain.Cargo{
					{ID: uuid.New(), Code: "C-1", Status: domain.CargoStatusPending},
					{ID: uuid.New(), Code: "C-2", Status: domain.CargoStatusPending},
				}, 12, nil
			},
		}
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes?status=pending&limit=5&offset=2", "")

		require.Equal(t, http.StatusOK, recorder.Code)

		var response struct {
			Items  []map[string]any `json:"items"`
			Total  int              `json:"total"`
			Limit  int              `json:"limit"`
			Offset int              `json:"offset"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		assert.Len(t, response.Items, 2)
		assert.Equal(t, 12, response.Total)
		assert.Equal(t, 5, response.Limit)
		assert.Equal(t, 2, response.Offset)
	})

	t.Run("rejects an invalid status", func(t *testing.T) {
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, &fakeTrackingService{}), http.MethodGet, "/api/v1/cargoes?status=lost", "")

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestCargoHandlerChangeStatus(t *testing.T) {
	cargoID := uuid.New()

	t.Run("changes the status", func(t *testing.T) {
		cargoService := &fakeCargoService{
			changeStatusFn: func(ctx context.Context, id uuid.UUID, next domain.CargoStatus, location string) (*domain.Cargo, error) {
				assert.Equal(t, cargoID, id)
				assert.Equal(t, domain.CargoStatusInTransit, next)
				assert.Equal(t, "Rotterdam", location)
				return &domain.Cargo{ID: id, Status: next}, nil
			},
		}
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPatch, "/api/v1/cargoes/"+cargoID.String()+"/status", `{"status":"in_transit","location":"Rotterdam"}`)

		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "in_transit")
	})

	t.Run("returns conflict for an invalid transition", func(t *testing.T) {
		cargoService := &fakeCargoService{
			changeStatusFn: func(ctx context.Context, id uuid.UUID, next domain.CargoStatus, location string) (*domain.Cargo, error) {
				return nil, fmt.Errorf("%w: bad transition", domain.ErrInvalidTransition)
			},
		}
		recorder := performRequest(testRouter(cargoService, &fakeVesselService{}, &fakeTrackingService{}), http.MethodPatch, "/api/v1/cargoes/"+cargoID.String()+"/status", `{"status":"delivered","location":"Rotterdam"}`)

		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "invalid_status_transition")
	})
}

func TestVesselHandler(t *testing.T) {
	vesselID := uuid.New()

	t.Run("creates a vessel", func(t *testing.T) {
		vesselService := &fakeVesselService{
			createFn: func(ctx context.Context, in service.CreateVesselInput) (*domain.Vessel, error) {
				assert.Equal(t, "MV Test", in.Name)
				return &domain.Vessel{ID: vesselID, Name: in.Name, Capacity: in.Capacity, CurrentLocation: in.CurrentLocation}, nil
			},
		}
		recorder := performRequest(testRouter(&fakeCargoService{}, vesselService, &fakeTrackingService{}), http.MethodPost, "/api/v1/vessels", `{"name":"MV Test","capacity":100,"current_location":"Rotterdam"}`)

		require.Equal(t, http.StatusCreated, recorder.Code)
		assert.NotEmpty(t, recorder.Header().Get("Location"))
	})

	t.Run("lists vessels", func(t *testing.T) {
		vesselService := &fakeVesselService{
			listFn: func(ctx context.Context) ([]domain.Vessel, error) {
				return []domain.Vessel{{ID: vesselID, Name: "MV Test"}}, nil
			},
		}
		recorder := performRequest(testRouter(&fakeCargoService{}, vesselService, &fakeTrackingService{}), http.MethodGet, "/api/v1/vessels", "")

		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "MV Test")
	})

	t.Run("gets a vessel", func(t *testing.T) {
		vesselService := &fakeVesselService{
			getFn: func(ctx context.Context, id uuid.UUID) (*domain.Vessel, error) {
				return &domain.Vessel{ID: id, Name: "MV Test", Capacity: 100, CurrentLocation: "Rotterdam"}, nil
			},
		}
		recorder := performRequest(testRouter(&fakeCargoService{}, vesselService, &fakeTrackingService{}), http.MethodGet, "/api/v1/vessels/"+vesselID.String(), "")

		require.Equal(t, http.StatusOK, recorder.Code)
	})
}

func TestTrackingHandler(t *testing.T) {
	cargoID := uuid.New()

	t.Run("lists tracking history", func(t *testing.T) {
		trackingService := &fakeTrackingService{
			listFn: func(ctx context.Context, id uuid.UUID) ([]domain.TrackingEntry, error) {
				assert.Equal(t, cargoID, id)
				return []domain.TrackingEntry{{ID: uuid.New(), CargoID: id, Location: "Rotterdam", Status: domain.CargoStatusPending}}, nil
			},
		}
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, trackingService), http.MethodGet, "/api/v1/cargoes/"+cargoID.String()+"/tracking", "")

		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Rotterdam")
	})

	t.Run("adds a tracking entry", func(t *testing.T) {
		trackingService := &fakeTrackingService{
			addFn: func(ctx context.Context, id uuid.UUID, location string, occurredAt time.Time) (*domain.TrackingEntry, error) {
				assert.Equal(t, cargoID, id)
				assert.Equal(t, "Hamburg", location)
				return &domain.TrackingEntry{ID: uuid.New(), CargoID: id, Location: location, Status: domain.CargoStatusInTransit}, nil
			},
		}
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, trackingService), http.MethodPost, "/api/v1/cargoes/"+cargoID.String()+"/tracking", `{"location":"Hamburg"}`)

		require.Equal(t, http.StatusCreated, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Hamburg")
	})

	t.Run("returns validation errors", func(t *testing.T) {
		trackingService := &fakeTrackingService{
			addFn: func(ctx context.Context, id uuid.UUID, location string, occurredAt time.Time) (*domain.TrackingEntry, error) {
				return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "location", Message: "is required"}}}
			},
		}
		recorder := performRequest(testRouter(&fakeCargoService{}, &fakeVesselService{}, trackingService), http.MethodPost, "/api/v1/cargoes/"+cargoID.String()+"/tracking", `{"location":""}`)

		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	})
}
