package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

const vesselColumns = `id, name, capacity, current_location, created_at, updated_at`

type VesselRepository struct {
	pool *pgxpool.Pool
}

func NewVesselRepository(pool *pgxpool.Pool) *VesselRepository {
	return &VesselRepository{pool: pool}
}

func (r *VesselRepository) Create(ctx context.Context, vessel *domain.Vessel) error {
	const query = `
		INSERT INTO vessels (id, name, capacity, current_location, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.pool.Exec(ctx, query,
		pgUUID(vessel.ID),
		vessel.Name,
		vessel.Capacity,
		vessel.CurrentLocation,
		vessel.CreatedAt,
		vessel.UpdatedAt,
	)
	if err != nil {
		return mapWriteError(err, "vessel")
	}

	return nil
}

func (r *VesselRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Vessel, error) {
	query := `SELECT ` + vesselColumns + ` FROM vessels WHERE id = $1`

	vessel, err := scanVessel(r.pool.QueryRow(ctx, query, pgUUID(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: vessel %s", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("query vessel: %w", err)
	}

	return vessel, nil
}

func (r *VesselRepository) List(ctx context.Context) ([]domain.Vessel, error) {
	query := `SELECT ` + vesselColumns + ` FROM vessels ORDER BY name ASC, id ASC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query vessels: %w", err)
	}
	defer rows.Close()

	var vessels []domain.Vessel
	for rows.Next() {
		vessel, err := scanVessel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan vessel: %w", err)
		}
		vessels = append(vessels, *vessel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vessels: %w", err)
	}

	return vessels, nil
}

func (r *VesselRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vessels WHERE id = $1)`, pgUUID(id)).Scan(&exists); err != nil {
		return false, fmt.Errorf("check vessel existence: %w", err)
	}

	return exists, nil
}

func scanVessel(row pgx.Row) (*domain.Vessel, error) {
	var (
		vessel domain.Vessel
		id     pgtype.UUID
	)

	if err := row.Scan(&id, &vessel.Name, &vessel.Capacity, &vessel.CurrentLocation, &vessel.CreatedAt, &vessel.UpdatedAt); err != nil {
		return nil, err
	}

	vessel.ID = uuidFromPG(id)

	return &vessel, nil
}
