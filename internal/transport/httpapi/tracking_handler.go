package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

type TrackingHandler struct {
	service TrackingService
	logger  *slog.Logger
}

func NewTrackingHandler(service TrackingService, logger *slog.Logger) *TrackingHandler {
	return &TrackingHandler{service: service, logger: logger}
}

func (h *TrackingHandler) List(w http.ResponseWriter, r *http.Request) {
	cargoID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, r, h.logger, badRequest("id must be a valid UUID"))
		return
	}

	entries, err := h.service.List(r.Context(), cargoID)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	items := make([]trackingResponse, 0, len(entries))
	for i := range entries {
		items = append(items, newTrackingResponse(&entries[i]))
	}

	writeJSON(w, http.StatusOK, trackingListResponse{Items: items, Total: len(items)})
}

func (h *TrackingHandler) Add(w http.ResponseWriter, r *http.Request) {
	cargoID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, r, h.logger, badRequest("id must be a valid UUID"))
		return
	}

	var request addTrackingRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, r, h.logger, badRequest("%s", err))
		return
	}

	entry, err := h.service.Add(r.Context(), cargoID, request.Location, request.OccurredAt)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusCreated, newTrackingResponse(entry))
}
