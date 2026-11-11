package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

type TrackingService struct {
	tracking TrackingRepository
	cargos   CargoRepository
}

func NewTrackingService(tracking TrackingRepository, cargos CargoRepository) *TrackingService {
	return &TrackingService{tracking: tracking, cargos: cargos}
}

func (s *TrackingService) List(ctx context.Context, cargoID uuid.UUID) ([]domain.TrackingEntry, error) {
	if _, err := s.cargos.GetByID(ctx, cargoID); err != nil {
		return nil, fmt.Errorf("get cargo: %w", err)
	}

	entries, err := s.tracking.ListByCargoID(ctx, cargoID)
	if err != nil {
		return nil, fmt.Errorf("list tracking entries: %w", err)
	}

	return entries, nil
}

func (s *TrackingService) Add(ctx context.Context, cargoID uuid.UUID, location string, occurredAt time.Time) (*domain.TrackingEntry, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil, &domain.ValidationError{Fields: []domain.FieldError{
			{Field: "location", Message: "is required"},
		}}
	}

	cargo, err := s.cargos.GetByID(ctx, cargoID)
	if err != nil {
		return nil, fmt.Errorf("get cargo: %w", err)
	}

	now := time.Now().UTC()
	if occurredAt.IsZero() {
		occurredAt = now
	}

	entry := &domain.TrackingEntry{
		ID:         uuid.New(),
		CargoID:    cargoID,
		Location:   location,
		Status:     cargo.Status,
		OccurredAt: occurredAt.UTC(),
		CreatedAt:  now,
	}

	if err := s.tracking.Create(ctx, entry); err != nil {
		return nil, fmt.Errorf("create tracking entry: %w", err)
	}

	return entry, nil
}
