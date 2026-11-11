package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

type CreateCargoInput struct {
	Code        string
	Description string
	WeightKg    float64
	VesselID    uuid.UUID
}

type CargoService struct {
	cargos    CargoRepository
	vessels   VesselRepository
	publisher EventPublisher
	logger    *slog.Logger
}

func NewCargoService(cargos CargoRepository, vessels VesselRepository, publisher EventPublisher, logger *slog.Logger) *CargoService {
	return &CargoService{
		cargos:    cargos,
		vessels:   vessels,
		publisher: publisher,
		logger:    logger,
	}
}

func (s *CargoService) Create(ctx context.Context, in CreateCargoInput) (*domain.Cargo, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Description = strings.TrimSpace(in.Description)

	var fields []domain.FieldError
	switch {
	case in.Code == "":
		fields = append(fields, domain.FieldError{Field: "code", Message: "is required"})
	case len(in.Code) > 64:
		fields = append(fields, domain.FieldError{Field: "code", Message: "must be at most 64 characters"})
	}
	if in.WeightKg <= 0 {
		fields = append(fields, domain.FieldError{Field: "weight_kg", Message: "must be greater than zero"})
	}
	if in.VesselID == uuid.Nil {
		fields = append(fields, domain.FieldError{Field: "vessel_id", Message: "is required"})
	}
	if len(fields) > 0 {
		return nil, &domain.ValidationError{Fields: fields}
	}

	exists, err := s.vessels.Exists(ctx, in.VesselID)
	if err != nil {
		return nil, fmt.Errorf("check vessel: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: vessel %s", domain.ErrNotFound, in.VesselID)
	}

	now := time.Now().UTC()
	cargo := &domain.Cargo{
		ID:          uuid.New(),
		Code:        in.Code,
		Description: in.Description,
		WeightKg:    in.WeightKg,
		Status:      domain.CargoStatusPending,
		VesselID:    in.VesselID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.cargos.Create(ctx, cargo); err != nil {
		return nil, fmt.Errorf("create cargo: %w", err)
	}

	return cargo, nil
}

func (s *CargoService) Get(ctx context.Context, id uuid.UUID) (*domain.Cargo, error) {
	cargo, err := s.cargos.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get cargo: %w", err)
	}

	return cargo, nil
}

func (s *CargoService) List(ctx context.Context, filter domain.CargoFilter) ([]domain.Cargo, int, error) {
	if filter.Status != nil && !filter.Status.Valid() {
		return nil, 0, &domain.ValidationError{Fields: []domain.FieldError{
			{Field: "status", Message: "must be one of pending, in_transit, delivered"},
		}}
	}

	if filter.Limit <= 0 {
		filter.Limit = defaultLimit
	}
	if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	cargos, total, err := s.cargos.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("list cargoes: %w", err)
	}

	return cargos, total, nil
}

func (s *CargoService) ChangeStatus(ctx context.Context, id uuid.UUID, next domain.CargoStatus, location string) (*domain.Cargo, error) {
	location = strings.TrimSpace(location)

	var fields []domain.FieldError
	if !next.Valid() {
		fields = append(fields, domain.FieldError{Field: "status", Message: "must be one of pending, in_transit, delivered"})
	}
	if location == "" {
		fields = append(fields, domain.FieldError{Field: "location", Message: "is required"})
	}
	if len(fields) > 0 {
		return nil, &domain.ValidationError{Fields: fields}
	}

	current, err := s.cargos.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get cargo: %w", err)
	}

	if !current.Status.CanTransitionTo(next) {
		return nil, fmt.Errorf("%w: cargo %s cannot move from %s to %s", domain.ErrInvalidTransition, id, current.Status, next)
	}

	now := time.Now().UTC()
	entry := &domain.TrackingEntry{
		ID:         uuid.New(),
		CargoID:    id,
		Location:   location,
		Status:     next,
		OccurredAt: now,
		CreatedAt:  now,
	}

	updated, err := s.cargos.ChangeStatus(ctx, id, current.Status, next, entry)
	if err != nil {
		return nil, fmt.Errorf("change cargo status: %w", err)
	}

	event := domain.CargoStatusChanged{
		EventID:    uuid.New(),
		CargoID:    id,
		OldStatus:  current.Status,
		NewStatus:  next,
		OccurredAt: now,
	}
	if err := s.publisher.PublishCargoStatusChanged(ctx, event); err != nil {
		// The status change is already committed. Failing the request would be
		// misleading, so record the publish failure for operators to retry.
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "failed to publish cargo status changed event",
				"cargo_id", id.String(),
				"event_id", event.EventID.String(),
				"error", err,
			)
		}
	}

	return updated, nil
}
