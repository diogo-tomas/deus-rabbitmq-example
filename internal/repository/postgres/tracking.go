package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

type TrackingRepository struct {
	pool *pgxpool.Pool
}

func NewTrackingRepository(pool *pgxpool.Pool) *TrackingRepository {
	return &TrackingRepository{pool: pool}
}

func (r *TrackingRepository) Create(ctx context.Context, entry *domain.TrackingEntry) error {
	const query = `
		INSERT INTO tracking (id, cargo_id, location, status, occurred_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.pool.Exec(ctx, query,
		pgUUID(entry.ID),
		pgUUID(entry.CargoID),
		entry.Location,
		string(entry.Status),
		entry.OccurredAt,
		entry.CreatedAt,
	)
	if err != nil {
		return mapWriteError(err, "tracking entry")
	}

	return nil
}

func (r *TrackingRepository) ListByCargoID(ctx context.Context, cargoID uuid.UUID) ([]domain.TrackingEntry, error) {
	const query = `
		SELECT id, cargo_id, location, status, occurred_at, created_at
		FROM tracking
		WHERE cargo_id = $1
		ORDER BY occurred_at DESC, created_at DESC`

	rows, err := r.pool.Query(ctx, query, pgUUID(cargoID))
	if err != nil {
		return nil, fmt.Errorf("query tracking entries: %w", err)
	}
	defer rows.Close()

	var entries []domain.TrackingEntry
	for rows.Next() {
		var (
			entry  domain.TrackingEntry
			id     pgtype.UUID
			cargo  pgtype.UUID
			status string
		)

		if err := rows.Scan(&id, &cargo, &entry.Location, &status, &entry.OccurredAt, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tracking entry: %w", err)
		}

		entry.ID = uuidFromPG(id)
		entry.CargoID = uuidFromPG(cargo)
		entry.Status = domain.CargoStatus(status)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tracking entries: %w", err)
	}

	return entries, nil
}
