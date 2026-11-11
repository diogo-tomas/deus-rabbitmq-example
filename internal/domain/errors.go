package domain

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrValidation        = errors.New("validation failed")
)

// FieldError describes a single invalid input field.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError groups one or more field validation failures.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return ErrValidation.Error()
	}

	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Field, f.Message))
	}

	return ErrValidation.Error() + ": " + strings.Join(parts, "; ")
}

func (e *ValidationError) Unwrap() error { return ErrValidation }
