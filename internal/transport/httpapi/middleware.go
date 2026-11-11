package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const requestIDContextKey contextKey = "request_id"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}

		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDContextKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey).(string)
	return id
}

func Logger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		if logger != nil {
			logger.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", requestIDFromContext(r.Context()),
				"remote_addr", r.RemoteAddr,
			)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wrote {
		return
	}

	r.wrote = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}

	return r.ResponseWriter.Write(body)
}

func Recovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if logger != nil {
					logger.ErrorContext(r.Context(), "panic recovered",
						"panic", recovered,
						"request_id", requestIDFromContext(r.Context()),
					)
				}

				writeJSON(w, http.StatusInternalServerError, errorEnvelope{Error: errorDetail{
					Code:      "internal_error",
					Message:   "internal server error",
					RequestID: requestIDFromContext(r.Context()),
				}})
			}
		}()

		next.ServeHTTP(w, r)
	})
}
