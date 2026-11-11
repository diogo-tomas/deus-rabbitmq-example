package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

type badRequestError struct {
	message string
}

func (e *badRequestError) Error() string { return e.message }

func badRequest(format string, args ...any) error {
	return &badRequestError{message: fmt.Sprintf(format, args...)}
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string             `json:"code"`
	Message   string             `json:"message"`
	Fields    []fieldErrorDetail `json:"fields,omitempty"`
	RequestID string             `json:"request_id,omitempty"`
}

type fieldErrorDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	detail := errorDetail{
		Code:      "internal_error",
		Message:   "internal server error",
		RequestID: requestIDFromContext(r.Context()),
	}

	var (
		validation *domain.ValidationError
		badReq     *badRequestError
	)

	switch {
	case errors.As(err, &badReq):
		detail.Code = "bad_request"
		detail.Message = badReq.Error()
	case errors.As(err, &validation):
		detail.Code = "validation_error"
		detail.Message = "request validation failed"
		detail.Fields = make([]fieldErrorDetail, 0, len(validation.Fields))
		for _, field := range validation.Fields {
			detail.Fields = append(detail.Fields, fieldErrorDetail{
				Field:   field.Field,
				Message: field.Message,
			})
		}
	case errors.Is(err, domain.ErrValidation):
		detail.Code = "validation_error"
		detail.Message = err.Error()
	case errors.Is(err, domain.ErrNotFound):
		detail.Code = "not_found"
		detail.Message = err.Error()
	case errors.Is(err, domain.ErrInvalidTransition):
		detail.Code = "invalid_status_transition"
		detail.Message = err.Error()
	case errors.Is(err, domain.ErrConflict):
		detail.Code = "conflict"
		detail.Message = err.Error()
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "request failed", "error", err)
		}
	}

	status := http.StatusInternalServerError
	switch detail.Code {
	case "bad_request":
		status = http.StatusBadRequest
	case "validation_error":
		status = http.StatusUnprocessableEntity
	case "not_found":
		status = http.StatusNotFound
	case "invalid_status_transition", "conflict":
		status = http.StatusConflict
	}

	writeJSON(w, status, errorEnvelope{Error: detail})
}
