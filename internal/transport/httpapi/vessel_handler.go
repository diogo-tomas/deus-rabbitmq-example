package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/example/deus-rabbitmq-example/internal/service"
)

type VesselHandler struct {
	service VesselService
	logger  *slog.Logger
}

func NewVesselHandler(service VesselService, logger *slog.Logger) *VesselHandler {
	return &VesselHandler{service: service, logger: logger}
}

func (h *VesselHandler) Create(w http.ResponseWriter, r *http.Request) {
	var request createVesselRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, h.logger, badRequest("%s", err))
		return
	}

	vessel, err := h.service.Create(r.Context(), service.CreateVesselInput{
		Name:            request.Name,
		Capacity:        request.Capacity,
		CurrentLocation: request.CurrentLocation,
	})
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	w.Header().Set("Location", "/api/v1/vessels/"+vessel.ID.String())
	writeJSON(w, http.StatusCreated, newVesselResponse(vessel))
}

func (h *VesselHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, r, h.logger, badRequest("id must be a valid UUID"))
		return
	}

	vessel, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusOK, newVesselResponse(vessel))
}

func (h *VesselHandler) List(w http.ResponseWriter, r *http.Request) {
	vessels, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	items := make([]vesselResponse, 0, len(vessels))
	for i := range vessels {
		items = append(items, newVesselResponse(&vessels[i]))
	}

	writeJSON(w, http.StatusOK, vesselListResponse{Items: items, Total: len(items)})
}
