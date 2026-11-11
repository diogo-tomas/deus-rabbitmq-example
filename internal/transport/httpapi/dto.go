package httpapi

import (
	"time"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

type createCargoRequest struct {
	Code        string  `json:"code"`
	Description string  `json:"description"`
	WeightKg    float64 `json:"weight_kg"`
	VesselID    string  `json:"vessel_id"`
}

type changeCargoStatusRequest struct {
	Status   string `json:"status"`
	Location string `json:"location"`
}

type cargoResponse struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Description string    `json:"description"`
	WeightKg    float64   `json:"weight_kg"`
	Status      string    `json:"status"`
	VesselID    string    `json:"vessel_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type cargoListResponse struct {
	Items  []cargoResponse `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

func newCargoResponse(cargo *domain.Cargo) cargoResponse {
	return cargoResponse{
		ID:          cargo.ID.String(),
		Code:        cargo.Code,
		Description: cargo.Description,
		WeightKg:    cargo.WeightKg,
		Status:      string(cargo.Status),
		VesselID:    cargo.VesselID.String(),
		CreatedAt:   cargo.CreatedAt,
		UpdatedAt:   cargo.UpdatedAt,
	}
}

type createVesselRequest struct {
	Name            string `json:"name"`
	Capacity        int    `json:"capacity"`
	CurrentLocation string `json:"current_location"`
}

type vesselResponse struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Capacity        int       `json:"capacity"`
	CurrentLocation string    `json:"current_location"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type vesselListResponse struct {
	Items []vesselResponse `json:"items"`
	Total int              `json:"total"`
}

func newVesselResponse(vessel *domain.Vessel) vesselResponse {
	return vesselResponse{
		ID:              vessel.ID.String(),
		Name:            vessel.Name,
		Capacity:        vessel.Capacity,
		CurrentLocation: vessel.CurrentLocation,
		CreatedAt:       vessel.CreatedAt,
		UpdatedAt:       vessel.UpdatedAt,
	}
}

type addTrackingRequest struct {
	Location   string    `json:"location"`
	OccurredAt time.Time `json:"occurred_at"`
}

type trackingResponse struct {
	ID         string    `json:"id"`
	CargoID    string    `json:"cargo_id"`
	Location   string    `json:"location"`
	Status     string    `json:"status"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type trackingListResponse struct {
	Items []trackingResponse `json:"items"`
	Total int                `json:"total"`
}

func newTrackingResponse(entry *domain.TrackingEntry) trackingResponse {
	return trackingResponse{
		ID:         entry.ID.String(),
		CargoID:    entry.CargoID.String(),
		Location:   entry.Location,
		Status:     string(entry.Status),
		OccurredAt: entry.OccurredAt,
		CreatedAt:  entry.CreatedAt,
	}
}
