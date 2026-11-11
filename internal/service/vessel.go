package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

type CreateVesselInput struct {
	Name            string
	Capacity        int
	CurrentLocation string
}

type VesselService struct {
	vessels VesselRepository
}

func NewVesselService(vessels VesselRepository) *VesselService {
	return &VesselService{vessels: vessels}
}

func (s *VesselService) Create(ctx context.Context, in CreateVesselInput) (*domain.Vessel, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.CurrentLocation = strings.TrimSpace(in.CurrentLocation)

	var fields []domain.FieldError
	switch {
	case in.Name == "":
		fields = append(fields, domain.FieldError{Field: "name", Message: "is required"})
	case len(in.Name) > 128:
		fields = append(fields, domain.FieldError{Field: "name", Message: "must be at most 128 characters"})
	}
	if in.Capacity <= 0 {
		fields = append(fields, domain.FieldError{Field: "capacity", Message: "must be greater than zero"})
	}
	if in.CurrentLocation == "" {
		fields = append(fields, domain.FieldError{Field: "current_location", Message: "is required"})
	}
	if len(fields) > 0 {
		return nil, &domain.ValidationError{Fields: fields}
	}

	now := time.Now().UTC()
	vessel := &domain.Vessel{
		ID:              uuid.New(),
		Name:            in.Name,
		Capacity:        in.Capacity,
		CurrentLocation: in.CurrentLocation,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.vessels.Create(ctx, vessel); err != nil {
		return nil, fmt.Errorf("create vessel: %w", err)
	}

	return vessel, nil
}

func (s *VesselService) Get(ctx context.Context, id uuid.UUID) (*domain.Vessel, error) {
	vessel, err := s.vessels.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get vessel: %w", err)
	}

	return vessel, nil
}

func (s *VesselService) List(ctx context.Context) ([]domain.Vessel, error) {
	vessels, err := s.vessels.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list vessels: %w", err)
	}

	return vessels, nil
}
