package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Pinger is implemented by the database pool and used by the readiness probe.
type Pinger interface {
	Ping(ctx context.Context) error
}

func NewRouter(cargo *CargoHandler, vessel *VesselHandler, tracking *TrackingHandler, pinger Pinger, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if pinger != nil {
			if err := pinger.Ping(ctx); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/vessels", vessel.Create)
	mux.HandleFunc("GET /api/v1/vessels", vessel.List)
	mux.HandleFunc("GET /api/v1/vessels/{id}", vessel.Get)

	mux.HandleFunc("POST /api/v1/cargoes", cargo.Create)
	mux.HandleFunc("GET /api/v1/cargoes", cargo.List)
	mux.HandleFunc("GET /api/v1/cargoes/{id}", cargo.Get)
	mux.HandleFunc("PATCH /api/v1/cargoes/{id}/status", cargo.ChangeStatus)

	mux.HandleFunc("GET /api/v1/cargoes/{id}/tracking", tracking.List)
	mux.HandleFunc("POST /api/v1/cargoes/{id}/tracking", tracking.Add)

	return RequestID(Logger(logger, Recovery(logger, mux)))
}
