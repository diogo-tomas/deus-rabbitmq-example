package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/example/deus-rabbitmq-example/internal/domain"
	"github.com/example/deus-rabbitmq-example/internal/service"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

type CargoHandler struct {
	service CargoService
	logger  *slog.Logger
}

func NewCargoHandler(service CargoService, logger *slog.Logger) *CargoHandler {
	return &CargoHandler{service: service, logger: logger}
}

func (h *CargoHandler) Create(w http.ResponseWriter, r *http.Request) {
	var request createCargoRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, h.logger, badRequest("%s", err))
		return
	}

	vesselID, err := uuid.Parse(request.VesselID)
	if err != nil {
		writeError(w, r, h.logger, badRequest("vessel_id must be a valid UUID"))
		return
	}

	cargo, err := h.service.Create(r.Context(), service.CreateCargoInput{
		Code:        request.Code,
		Description: request.Description,
		WeightKg:    request.WeightKg,
		VesselID:    vesselID,
	})
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	w.Header().Set("Location", "/api/v1/cargoes/"+cargo.ID.String())
	writeJSON(w, http.StatusCreated, newCargoResponse(cargo))
}

func (h *CargoHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, r, h.logger, badRequest("id must be a valid UUID"))
		return
	}

	cargo, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusOK, newCargoResponse(cargo))
}

func (h *CargoHandler) List(w http.ResponseWriter, r *http.Request) {
	filter, err := parseCargoFilter(r)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	cargos, total, err := h.service.List(r.Context(), filter)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	items := make([]cargoResponse, 0, len(cargos))
	for i := range cargos {
		items = append(items, newCargoResponse(&cargos[i]))
	}

	writeJSON(w, http.StatusOK, cargoListResponse{
		Items:  items,
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

func (h *CargoHandler) ChangeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, r, h.logger, badRequest("id must be a valid UUID"))
		return
	}

	var request changeCargoStatusRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, h.logger, badRequest("%s", err))
		return
	}

	cargo, err := h.service.ChangeStatus(r.Context(), id, domain.CargoStatus(request.Status), request.Location)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusOK, newCargoResponse(cargo))
}

func parseCargoFilter(r *http.Request) (domain.CargoFilter, error) {
	filter := domain.CargoFilter{Limit: defaultPageLimit}

	if raw := r.URL.Query().Get("status"); raw != "" {
		status := domain.CargoStatus(raw)
		if !status.Valid() {
			return filter, badRequest("status must be one of pending, in_transit, delivered")
		}
		filter.Status = &status
	}

	if raw := r.URL.Query().Get("vessel_id"); raw != "" {
		vesselID, err := uuid.Parse(raw)
		if err != nil {
			return filter, badRequest("vessel_id must be a valid UUID")
		}
		filter.VesselID = &vesselID
	}

	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return filter, badRequest("limit must be a positive integer")
		}
		if limit > maxPageLimit {
			limit = maxPageLimit
		}
		filter.Limit = limit
	}

	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return filter, badRequest("offset must be a non-negative integer")
		}
		filter.Offset = offset
	}

	return filter, nil
}
