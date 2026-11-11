package migrations

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed *.sql
var files embed.FS

func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	source, err := iofs.New(files, ".")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load migrations: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migrator: %w", err)
	}

	return m, nil
}

// Up applies all pending migrations.
func Up(databaseURL string) error {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

// Down rolls back all migrations.
func Down(databaseURL string) error {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("rollback migrations: %w", err)
	}

	return nil
}

// Version reports the current migration version and dirty flag.
func Version(databaseURL string) (uint, bool, error) {
	m, err := newMigrator(databaseURL)
	if err != nil {
		return 0, false, err
	}
	defer func() { _, _ = m.Close() }()

	return m.Version()
}
