package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

const cargoColumns = `id, code, description, weight_kg, status, vessel_id, created_at, updated_at`

type CargoRepository struct {
	pool *pgxpool.Pool
}

func NewCargoRepository(pool *pgxpool.Pool) *CargoRepository {
	return &CargoRepository{pool: pool}
}

func (r *CargoRepository) Create(ctx context.Context, cargo *domain.Cargo) error {
	const query = `
		INSERT INTO cargoes (id, code, description, weight_kg, status, vessel_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.pool.Exec(ctx, query,
		pgUUID(cargo.ID),
		cargo.Code,
		cargo.Description,
		cargo.WeightKg,
		string(cargo.Status),
		pgUUID(cargo.VesselID),
		cargo.CreatedAt,
		cargo.UpdatedAt,
	)
	if err != nil {
		return mapWriteError(err, "cargo")
	}

	return nil
}

func (r *CargoRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Cargo, error) {
	query := `SELECT ` + cargoColumns + ` FROM cargoes WHERE id = $1`

	cargo, err := scanCargo(r.pool.QueryRow(ctx, query, pgUUID(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: cargo %s", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("query cargo: %w", err)
	}

	return cargo, nil
}

func (r *CargoRepository) List(ctx context.Context, filter domain.CargoFilter) ([]domain.Cargo, int, error) {
	var (
		conditions []string
		args       []any
	)

	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if filter.Status != nil {
		conditions = append(conditions, "status = "+addArg(string(*filter.Status)))
	}
	if filter.VesselID != nil {
		conditions = append(conditions, "vessel_id = "+addArg(pgUUID(*filter.VesselID)))
	}

	query := `SELECT ` + cargoColumns + `, count(*) OVER() AS total FROM cargoes`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC, id DESC"
	query += " LIMIT " + addArg(filter.Limit) + " OFFSET " + addArg(filter.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query cargoes: %w", err)
	}
	defer rows.Close()

	var (
		cargos []domain.Cargo
		total  int64
	)

	for rows.Next() {
		var (
			cargo    domain.Cargo
			id       pgtype.UUID
			vesselID pgtype.UUID
			status   string
		)

		if err := rows.Scan(&id, &cargo.Code, &cargo.Description, &cargo.WeightKg, &status, &vesselID, &cargo.CreatedAt, &cargo.UpdatedAt, &total); err != nil {
			return nil, 0, fmt.Errorf("scan cargo: %w", err)
		}

		cargo.ID = uuidFromPG(id)
		cargo.VesselID = uuidFromPG(vesselID)
		cargo.Status = domain.CargoStatus(status)
		cargos = append(cargos, cargo)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cargoes: %w", err)
	}

	return cargos, int(total), nil
}

// ChangeStatus atomically updates the cargo status and appends a tracking row.
// It locks the cargo row, re-checks the expected status, and commits both writes
// together. The service validates business transitions; this method protects
// against lost updates from concurrent requests.
func (r *CargoRepository) ChangeStatus(ctx context.Context, id uuid.UUID, from, to domain.CargoStatus, tracking *domain.TrackingEntry) (*domain.Cargo, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current string
	err = tx.QueryRow(ctx, `SELECT status FROM cargoes WHERE id = $1 FOR UPDATE`, pgUUID(id)).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: cargo %s", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("lock cargo: %w", err)
	}
	if current != string(from) {
		return nil, fmt.Errorf("%w: cargo %s status is %s, expected %s", domain.ErrConflict, id, current, from)
	}

	query := `UPDATE cargoes SET status = $1, updated_at = $2 WHERE id = $3 RETURNING ` + cargoColumns
	updated, err := scanCargo(tx.QueryRow(ctx, query, string(to), tracking.CreatedAt, pgUUID(id)))
	if err != nil {
		return nil, fmt.Errorf("update cargo status: %w", err)
	}

	const insertTracking = `
		INSERT INTO tracking (id, cargo_id, location, status, occurred_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := tx.Exec(ctx, insertTracking,
		pgUUID(tracking.ID),
		pgUUID(tracking.CargoID),
		tracking.Location,
		string(tracking.Status),
		tracking.OccurredAt,
		tracking.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("insert tracking entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return updated, nil
}

func scanCargo(row pgx.Row) (*domain.Cargo, error) {
	var (
		cargo    domain.Cargo
		id       pgtype.UUID
		vesselID pgtype.UUID
		status   string
	)

	if err := row.Scan(&id, &cargo.Code, &cargo.Description, &cargo.WeightKg, &status, &vesselID, &cargo.CreatedAt, &cargo.UpdatedAt); err != nil {
		return nil, err
	}

	cargo.ID = uuidFromPG(id)
	cargo.VesselID = uuidFromPG(vesselID)
	cargo.Status = domain.CargoStatus(status)

	return &cargo, nil
}
