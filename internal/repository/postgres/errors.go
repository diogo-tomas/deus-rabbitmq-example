package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/example/deus-rabbitmq-example/internal/domain"
)

// mapWriteError translates common PostgreSQL constraint violations into domain
// errors so the service and transport layers do not depend on driver details.
func mapWriteError(err error, resource string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s already exists", domain.ErrConflict, resource)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: related %s does not exist", domain.ErrNotFound, resource)
		case "23514": // check_violation
			return fmt.Errorf("%w: %s violates a database constraint", domain.ErrValidation, resource)
		}
	}

	return err
}
