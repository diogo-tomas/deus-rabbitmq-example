// Command seed loads a small, realistic dataset into the database so the API can
// be explored interactively with curl, Postman, or the spec in openapi.yaml.
//
// The seeder writes through the repository layer rather than the HTTP API, so it
// does not need the server or the broker to be running. Historical movements are
// inserted with backdated timestamps, which is exactly what makes the tracking
// history interesting to look at.
//
// Seeding is idempotent: every row uses a UUID derived from a human-readable
// slug, and rows that already exist are skipped. Running `make seed` twice
// leaves the database unchanged.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"

	"github.com/example/deus-rabbitmq-example/internal/config"
	"github.com/example/deus-rabbitmq-example/internal/domain"
	"github.com/example/deus-rabbitmq-example/internal/platform/db"
	"github.com/example/deus-rabbitmq-example/internal/repository/postgres"
	"github.com/example/deus-rabbitmq-example/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.RunMigrations {
		if err := migrations.Up(cfg.DatabaseURL); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	s := &seeder{
		cargos:   postgres.NewCargoRepository(pool),
		vessels:  postgres.NewVesselRepository(pool),
		tracking: postgres.NewTrackingRepository(pool),
		events:   postgres.NewEventRepository(pool),
	}

	// A single timestamp anchors every generated row so the seed data stays
	// internally consistent even if the run takes a while.
	summary, err := s.seed(ctx, time.Now().UTC())
	if err != nil {
		return err
	}

	summary.print(os.Stdout)

	return nil
}

type seeder struct {
	cargos   *postgres.CargoRepository
	vessels  *postgres.VesselRepository
	tracking *postgres.TrackingRepository
	events   *postgres.EventRepository
}

// step is one movement in a cargo journey. Steps are listed oldest first.
type step struct {
	status   domain.CargoStatus
	location string
	ago      time.Duration
	// pingOnly appends a tracking row without changing the cargo status, the
	// way a location update works through POST /cargoes/{id}/tracking.
	pingOnly bool
}

type vesselSeed struct {
	slug     string
	name     string
	capacity int
	location string
}

type cargoSeed struct {
	slug        string
	code        string
	description string
	weightKg    float64
	vesselSlug  string
	steps       []step
}

// vesselSeeds uses capacity in TEU (twenty-foot equivalent units), the unit the
// API leaves unspecified; cargo weights are per-shipment.
var vesselSeeds = []vesselSeed{
	{slug: "atlantic-voyager", name: "MV Atlantic Voyager", capacity: 12000, location: "Singapore, SG"},
	{slug: "pacific-meridian", name: "MV Pacific Meridian", capacity: 8500, location: "Kaohsiung, TW"},
	{slug: "baltic-star", name: "MV Baltic Star", capacity: 4200, location: "Gdansk, PL"},
}

var cargoSeeds = []cargoSeed{
	{
		slug:        "cargo-2024-001",
		code:        "CARGO-2024-001",
		description: "Refrigerated pharmaceuticals",
		weightKg:    18500.5,
		vesselSlug:  "atlantic-voyager",
		steps: []step{
			{status: domain.CargoStatusPending, location: "Hamburg, DE", ago: 168 * time.Hour},
			{status: domain.CargoStatusInTransit, location: "Hamburg, DE", ago: 150 * time.Hour},
			{status: domain.CargoStatusInTransit, location: "Rotterdam, NL", ago: 120 * time.Hour, pingOnly: true},
			{status: domain.CargoStatusInTransit, location: "Port Said, EG", ago: 72 * time.Hour, pingOnly: true},
			{status: domain.CargoStatusDelivered, location: "Singapore, SG", ago: 18 * time.Hour},
		},
	},
	{
		slug:        "cargo-2024-002",
		code:        "CARGO-2024-002",
		description: "Automotive spare parts",
		weightKg:    42000,
		vesselSlug:  "pacific-meridian",
		steps: []step{
			{status: domain.CargoStatusPending, location: "Yokohama, JP", ago: 96 * time.Hour},
			{status: domain.CargoStatusInTransit, location: "Yokohama, JP", ago: 80 * time.Hour},
			{status: domain.CargoStatusInTransit, location: "Kaohsiung, TW", ago: 40 * time.Hour, pingOnly: true},
		},
	},
	{
		slug:        "cargo-2024-003",
		code:        "CARGO-2024-003",
		description: "Cotton textiles",
		weightKg:    9800,
		vesselSlug:  "baltic-star",
		steps: []step{
			{status: domain.CargoStatusPending, location: "Gdansk, PL", ago: 6 * time.Hour},
		},
	},
	{
		slug:        "cargo-2024-004",
		code:        "CARGO-2024-004",
		description: "Lithium-ion battery packs",
		weightKg:    21000,
		vesselSlug:  "pacific-meridian",
		steps: []step{
			{status: domain.CargoStatusPending, location: "Busan, KR", ago: 240 * time.Hour},
			{status: domain.CargoStatusInTransit, location: "Busan, KR", ago: 220 * time.Hour},
			{status: domain.CargoStatusInTransit, location: "Shanghai, CN", ago: 190 * time.Hour, pingOnly: true},
			{status: domain.CargoStatusDelivered, location: "Los Angeles, US", ago: 60 * time.Hour},
		},
	},
	{
		slug:        "cargo-2024-005",
		code:        "CARGO-2024-005",
		description: "Industrial machine tools",
		weightKg:    33500,
		vesselSlug:  "atlantic-voyager",
		steps: []step{
			{status: domain.CargoStatusPending, location: "Antwerp, BE", ago: 60 * time.Hour},
			{status: domain.CargoStatusInTransit, location: "Antwerp, BE", ago: 48 * time.Hour},
		},
	},
}

// validate checks a journey against the domain rules before anything is written,
// so a typo in the seed table fails fast instead of leaving a half-built cargo.
func (s cargoSeed) validate() error {
	if len(s.steps) == 0 {
		return fmt.Errorf("cargo %q: no steps defined", s.code)
	}

	first := s.steps[0]
	if first.pingOnly || first.status != domain.CargoStatusPending {
		return fmt.Errorf("cargo %q: the first step must create the cargo in %q", s.code, domain.CargoStatusPending)
	}

	status := first.status
	previousAgo := first.ago
	for _, st := range s.steps[1:] {
		if st.ago >= previousAgo {
			return fmt.Errorf("cargo %q: step at %q is not older than the step before it", s.code, st.location)
		}
		previousAgo = st.ago

		if st.pingOnly {
			if st.status != status {
				return fmt.Errorf("cargo %q: tracking-only step at %q must keep status %q", s.code, st.location, status)
			}
			continue
		}

		if !status.CanTransitionTo(st.status) {
			return fmt.Errorf("cargo %q: cannot move from %q to %q", s.code, status, st.status)
		}
		status = st.status
	}

	return nil
}

type seededCargo struct {
	cargo      *domain.Cargo
	vesselName string
}

type summary struct {
	vesselsCreated int
	vesselsSkipped int
	cargoesCreated int
	cargoesSkipped int
	cargoes        []seededCargo
}

func (s *seeder) seed(ctx context.Context, now time.Time) (summary, error) {
	var sum summary

	for _, spec := range cargoSeeds {
		if err := spec.validate(); err != nil {
			return sum, err
		}
	}

	vessels := make(map[string]*domain.Vessel, len(vesselSeeds))
	for _, spec := range vesselSeeds {
		vessel, created, err := s.ensureVessel(ctx, now, spec)
		if err != nil {
			return sum, err
		}
		if created {
			sum.vesselsCreated++
		} else {
			sum.vesselsSkipped++
		}
		vessels[spec.slug] = vessel
	}

	for _, spec := range cargoSeeds {
		vessel, ok := vessels[spec.vesselSlug]
		if !ok {
			return sum, fmt.Errorf("cargo %q references unknown vessel slug %q", spec.code, spec.vesselSlug)
		}

		cargo, created, err := s.ensureCargo(ctx, now, spec, vessel)
		if err != nil {
			return sum, err
		}
		if created {
			sum.cargoesCreated++
		} else {
			sum.cargoesSkipped++
		}
		sum.cargoes = append(sum.cargoes, seededCargo{cargo: cargo, vesselName: vessel.Name})
	}

	return sum, nil
}

func (s *seeder) ensureVessel(ctx context.Context, now time.Time, spec vesselSeed) (*domain.Vessel, bool, error) {
	id := seedID("vessel", spec.slug)

	existing, err := s.vessels.GetByID(ctx, id)
	switch {
	case err == nil:
		return existing, false, nil
	case !errors.Is(err, domain.ErrNotFound):
		return nil, false, fmt.Errorf("look up vessel %q: %w", spec.name, err)
	}

	vessel := &domain.Vessel{
		ID:              id,
		Name:            spec.name,
		Capacity:        spec.capacity,
		CurrentLocation: spec.location,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.vessels.Create(ctx, vessel); err != nil {
		return nil, false, fmt.Errorf("create vessel %q: %w", spec.name, err)
	}

	return vessel, true, nil
}

func (s *seeder) ensureCargo(ctx context.Context, now time.Time, spec cargoSeed, vessel *domain.Vessel) (*domain.Cargo, bool, error) {
	id := seedID("cargo", spec.slug)

	existing, err := s.cargos.GetByID(ctx, id)
	switch {
	case err == nil:
		return existing, false, nil
	case !errors.Is(err, domain.ErrNotFound):
		return nil, false, fmt.Errorf("look up cargo %q: %w", spec.code, err)
	}

	createdAt := now.Add(-spec.steps[0].ago)
	cargo := &domain.Cargo{
		ID:          id,
		Code:        spec.code,
		Description: spec.description,
		WeightKg:    spec.weightKg,
		Status:      domain.CargoStatusPending,
		VesselID:    vessel.ID,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	if err := s.cargos.Create(ctx, cargo); err != nil {
		return nil, false, fmt.Errorf("create cargo %q: %w", spec.code, err)
	}

	// The origin booking is the same row a client appends with
	// POST /cargoes/{id}/tracking, so the history starts at the port of loading.
	origin := spec.steps[0]
	if err := s.tracking.Create(ctx, &domain.TrackingEntry{
		ID:         uuid.New(),
		CargoID:    id,
		Location:   origin.location,
		Status:     origin.status,
		OccurredAt: createdAt,
		CreatedAt:  createdAt,
	}); err != nil {
		return nil, false, fmt.Errorf("create origin tracking entry for %q: %w", spec.code, err)
	}

	status := domain.CargoStatusPending
	for _, st := range spec.steps[1:] {
		occurredAt := now.Add(-st.ago)

		entry := &domain.TrackingEntry{
			ID:         uuid.New(),
			CargoID:    id,
			Location:   st.location,
			Status:     status,
			OccurredAt: occurredAt,
			CreatedAt:  occurredAt,
		}

		if st.pingOnly {
			if err := s.tracking.Create(ctx, entry); err != nil {
				return nil, false, fmt.Errorf("create tracking entry for %q: %w", spec.code, err)
			}
			continue
		}

		entry.Status = st.status

		updated, err := s.cargos.ChangeStatus(ctx, id, status, st.status, entry)
		if err != nil {
			return nil, false, fmt.Errorf("move cargo %q to %s: %w", spec.code, st.status, err)
		}

		// Historical events go through the same repository the worker uses, so
		// the audit trail looks identical to event-driven production data
		// without requiring a broker during seeding.
		event := domain.CargoStatusChanged{
			EventID:    uuid.New(),
			CargoID:    id,
			OldStatus:  status,
			NewStatus:  st.status,
			OccurredAt: occurredAt,
		}
		if err := s.events.SaveCargoStatusChanged(ctx, event); err != nil {
			return nil, false, fmt.Errorf("save event for %q: %w", spec.code, err)
		}

		status = st.status
		cargo = updated
	}

	return cargo, true, nil
}

// seedID derives a stable UUID (version 5) from a slug. Deterministic IDs are
// what make a second run a no-op instead of a source of duplicate rows.
func seedID(kind, slug string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("deus-seed/v1/"+kind+"/"+slug))
}

func (s summary) print(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintf(tw, "Vessels: %d created, %d already present\n", s.vesselsCreated, s.vesselsSkipped)
	fmt.Fprintf(tw, "Cargoes: %d created, %d already present\n\n", s.cargoesCreated, s.cargoesSkipped)

	fmt.Fprintln(tw, "CODE\tSTATUS\tWEIGHT\tVESSEL\tID")
	for _, c := range s.cargoes {
		fmt.Fprintf(tw, "%s\t%s\t%.1f kg\t%s\t%s\n",
			c.cargo.Code, c.cargo.Status, c.cargo.WeightKg, c.vesselName, c.cargo.ID)
	}
	tw.Flush()

	var pendingID uuid.UUID
	for _, c := range s.cargoes {
		if c.cargo.Status == domain.CargoStatusPending {
			pendingID = c.cargo.ID
			break
		}
	}
	if pendingID == uuid.Nil {
		return
	}

	fmt.Fprintf(w, "\nTry it with the pending cargo %s:\n\n"+
		"  curl -sS 'http://localhost:8080/api/v1/cargoes?status=pending'\n"+
		"  curl -sS 'http://localhost:8080/api/v1/cargoes/%s/tracking'\n"+
		"  curl -sS -X PATCH 'http://localhost:8080/api/v1/cargoes/%s/status' \\\n"+
		"    -H 'Content-Type: application/json' \\\n"+
		"    -d '{\"status\":\"in_transit\",\"location\":\"Gdansk, PL\"}'\n\n"+
		"That last call publishes a real event, so cargo_events only grows while the worker runs.\n",
		pendingID, pendingID, pendingID)
}
