package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

func TestCargoStatusTransitions(t *testing.T) {
	tests := []struct {
		name string
		from domain.CargoStatus
		to   domain.CargoStatus
		want bool
	}{
		{"pending to in_transit", domain.CargoStatusPending, domain.CargoStatusInTransit, true},
		{"in_transit to delivered", domain.CargoStatusInTransit, domain.CargoStatusDelivered, true},
		{"pending to delivered", domain.CargoStatusPending, domain.CargoStatusDelivered, false},
		{"in_transit to pending", domain.CargoStatusInTransit, domain.CargoStatusPending, false},
		{"delivered to in_transit", domain.CargoStatusDelivered, domain.CargoStatusInTransit, false},
		{"pending to pending", domain.CargoStatusPending, domain.CargoStatusPending, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.from.CanTransitionTo(tt.to))
		})
	}
}

func TestCargoStatusValid(t *testing.T) {
	assert.True(t, domain.CargoStatusPending.Valid())
	assert.True(t, domain.CargoStatusInTransit.Valid())
	assert.True(t, domain.CargoStatusDelivered.Valid())
	assert.False(t, domain.CargoStatus("lost").Valid())
}

func TestValidationError(t *testing.T) {
	err := &domain.ValidationError{
		Fields: []domain.FieldError{
			{Field: "code", Message: "is required"},
			{Field: "weight_kg", Message: "must be greater than zero"},
		},
	}

	require.ErrorIs(t, err, domain.ErrValidation)
	assert.Contains(t, err.Error(), "code: is required")
	assert.Contains(t, err.Error(), "weight_kg: must be greater than zero")
}
