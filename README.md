# Deus Logistics API

Golang backend for the Deus coding challenge: manage cargoes, vessels, tracking history, and cargo status events.

The service exposes a versioned REST API, persists data in PostgreSQL, and publishes cargo status changes to RabbitMQ. A background worker consumes those events and stores them in `cargo_events` for auditability.

## Stack

| Concern | Choice |
|---|---|
| HTTP | Go standard library `net/http` with `ServeMux` method/path patterns |
| Logging | `log/slog` with JSON output |
| Database | PostgreSQL 16 + `jackc/pgx/v5` (`pgxpool`) |
| Migrations | `golang-migrate` with embedded SQL migrations |
| Queue | RabbitMQ + `rabbitmq/amqp091-go` |
| Tests | `testing`, `testify`, hand-written fakes, integration tests against a real Postgres |

## Architecture

Dependencies point in one direction:

```
transport/http  ->  service  ->  domain
                        ^
             repository/postgres / messaging/rabbitmq
```

- `domain` contains entities, status rules, and sentinel errors. It imports no infrastructure packages.
- `service` owns business rules. It declares the repository and publisher interfaces it needs.
- `repository/postgres` implements those interfaces using explicit SQL.
- `transport/http` handles JSON, validation-to-HTTP mapping, middleware, and routing.
- `messaging/rabbitmq` implements the event publisher and consumer.
- `cmd/*` wires concrete implementations together; there is no global container.

```
cmd/
  api/main.go              HTTP server, graceful shutdown
  worker/main.go           RabbitMQ consumer
  migrate/main.go          migration CLI (up/down/version)
internal/
  config/                  environment-based configuration
  domain/                  entities, status state machine, domain errors
  messaging/rabbitmq/      publisher, consumer, topology, event envelope
  platform/db/             pgx pool
  platform/logging/        slog JSON logger
  repository/postgres/     SQL repositories
  service/                 business logic + ports
  transport/http/          handlers, DTOs, middleware, router
migrations/                embedded SQL migrations
openapi.yaml               OpenAPI 3.0.3 specification
```

## Data model

```
vessels 1 ──── * cargoes 1 ──── * tracking
                     │
                     └──── * cargo_events   (written by the consumer)
```

| Table | Purpose | Key constraints and indexes |
|---|---|---|
| `vessels` | Ships | `name` unique, `capacity > 0` |
| `cargoes` | Shipments | `code` unique, `status` enum check, `vessel_id` FK, indexes on vessel/status/created_at |
| `tracking` | Append-only movement history | `cargo_id` FK, index on `(cargo_id, occurred_at DESC)` |
| `cargo_events` | Consumed status-change events | `event_id` unique (consumer idempotency), index on `(cargo_id, occurred_at DESC)` |

Allowed status transitions:

```
pending -> in_transit -> delivered
```

Any other transition returns `409 Conflict`. Status updates and the corresponding tracking row are written in one transaction. The cargo row is locked with `SELECT ... FOR UPDATE` and the expected status is re-checked, so concurrent requests cannot silently overwrite each other.

## API

Base path: `/api/v1`.

| Method | Path | Description |
|---|---|---|
| `POST` | `/vessels` | Create a vessel |
| `GET` | `/vessels` | List vessels |
| `GET` | `/vessels/{id}` | Get a vessel |
| `POST` | `/cargoes` | Create a cargo in `pending` status |
| `GET` | `/cargoes` | List cargoes with filters and pagination |
| `GET` | `/cargoes/{id}` | Get a cargo |
| `PATCH` | `/cargoes/{id}/status` | Change cargo status |
| `GET` | `/cargoes/{id}/tracking` | Get tracking history |
| `POST` | `/cargoes/{id}/tracking` | Append a movement/location entry |
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/readyz` | Readiness probe (checks Postgres) |

### OpenAPI specification

The complete API contract is in [`openapi.yaml`](openapi.yaml) (OpenAPI 3.0.3). Import it directly into Postman, Insomnia, Bruno, or Swagger UI:

- **Postman**: Import → File → select `openapi.yaml`
- **Insomnia**: Import → From File → select `openapi.yaml`
- **Bruno**: Import Collection → OpenAPI → select `openapi.yaml`

The spec points at `http://localhost:8080` and includes request/response examples for every endpoint. Validate it with:

```bash
make openapi-lint
```

The target uses a locally installed `redocly` binary when available and falls back to `npx` for fresh clones. The specification is hand-written; keep it in sync when routes or DTOs change. Lint rule overrides live in `redocly.yaml`.

### Create a vessel

```bash
curl -sS -X POST http://localhost:8080/api/v1/vessels \
  -H 'Content-Type: application/json' \
  -d '{"name":"MV Atlantic","capacity":1200,"current_location":"Rotterdam"}'
```

```json
{
  "id": "0f1e2d3c-4b5a-4978-8a6b-5c4d3e2f1a09",
  "name": "MV Atlantic",
  "capacity": 1200,
  "current_location": "Rotterdam",
  "created_at": "2026-11-10T12:00:00Z",
  "updated_at": "2026-11-10T12:00:00Z"
}
```

### Create a cargo

```bash
curl -sS -X POST http://localhost:8080/api/v1/cargoes \
  -H 'Content-Type: application/json' \
  -d '{
    "code": "C-1001",
    "description": "Medical supplies",
    "weight_kg": 1250.5,
    "vessel_id": "0f1e2d3c-4b5a-4978-8a6b-5c4d3e2f1a09"
  }'
```

Returns `201 Created` with a `Location` header.

### List cargoes

```bash
curl -sS 'http://localhost:8080/api/v1/cargoes?status=in_transit&limit=20&offset=0'
```

Supported query parameters: `status`, `vessel_id`, `limit` (default 20, max 100), `offset`.

```json
{
  "items": [],
  "total": 0,
  "limit": 20,
  "offset": 0
}
```

### Change cargo status

```bash
curl -sS -X PATCH http://localhost:8080/api/v1/cargoes/<cargo-id>/status \
  -H 'Content-Type: application/json' \
  -d '{"status":"in_transit","location":"Rotterdam"}'
```

The status change also appends a `tracking` row and publishes a `cargo.status_changed` event.

### Tracking history

```bash
curl -sS http://localhost:8080/api/v1/cargoes/<cargo-id>/tracking
```

```json
{
  "items": [
    {
      "id": "7a1f...",
      "cargo_id": "1b2c...",
      "location": "Rotterdam",
      "status": "in_transit",
      "occurred_at": "2026-11-10T12:05:00Z",
      "created_at": "2026-11-10T12:05:00Z"
    }
  ],
  "total": 1
}
```

To append a movement without changing status:

```bash
curl -sS -X POST http://localhost:8080/api/v1/cargoes/<cargo-id>/tracking \
  -H 'Content-Type: application/json' \
  -d '{"location":"Hamburg","occurred_at":"2026-11-11T08:30:00Z"}'
```

### Errors

All errors use the same envelope:

```json
{
  "error": {
    "code": "validation_error",
    "message": "request validation failed",
    "fields": [
      {"field": "code", "message": "is required"}
    ],
    "request_id": "b0f1..."
  }
}
```

| Status | Meaning |
|---|---|
| `400` | Malformed JSON or invalid path/query parameter |
| `404` | Resource not found |
| `409` | Duplicate resource, stale concurrent update, or invalid status transition |
| `422` | Semantic validation failure |
| `500` | Unexpected server error (details are logged, not returned) |

## Running locally

Prerequisites: Go 1.24+, Docker, Docker Compose, Make.

```bash
make deps      # starts Postgres and RabbitMQ
make run       # starts the API on :8080
```

In another terminal:

```bash
make worker    # starts the RabbitMQ consumer
```

The API applies embedded migrations on startup by default. To run migrations explicitly instead:

```bash
make migrate-up
# and start the API with RUN_MIGRATIONS=false
```

## Running with Docker

```bash
make docker-run
```

This builds the image and starts:

- `postgres` on `localhost:5432`
- `rabbitmq` on `localhost:5672`, management UI on `http://localhost:15672` (`deus_user` / `deus_pass`)
- `api` on `http://localhost:8080`
- `worker` consuming cargo events

Stop and remove volumes:

```bash
make docker-down
```

## Testing

```bash
make test              # unit tests with -race
make test-integration  # repository tests against Postgres (starts compose deps)
make cover             # unit + integration coverage across internal packages
make lint              # go vet (+ staticcheck when installed)
```

Unit tests cover domain rules, service orchestration with fake repositories/publishers, HTTP handlers through the real router, and consumer delivery handling with a fake RabbitMQ acknowledger.

Integration tests are gated by the `integration` build tag and require `TEST_DATABASE_URL` and `TEST_RABBITMQ_URL`:

```bash
TEST_DATABASE_URL='postgres://deus:deus@localhost:5432/deus?sslmode=disable' \
TEST_RABBITMQ_URL='amqp://deus_user:deus_pass@localhost:5672/' \
  go test -tags=integration ./...
```

They run migrations, truncate tables between tests, and exercise a real publish/consume round-trip on isolated RabbitMQ exchange and queue names. Use a dedicated database, not a shared development one.

## Event flow

On a successful status change:

1. The cargo update and tracking insert commit in one transaction.
2. The API publishes a persistent JSON message to the `cargo.events` topic exchange with routing key `cargo.status.changed`.
3. The message includes `event_id`, `type`, `occurred_at`, and a payload with `cargo_id`, `old_status`, `new_status`.
4. The worker consumes with manual acknowledgements and a prefetch of 10.
5. The worker inserts into `cargo_events` using `ON CONFLICT (event_id) DO NOTHING`, making at-least-once delivery safe.
6. Malformed messages are dead-lettered to `cargo.events.dlq`; storage failures are requeued.

Event shape:

```json
{
  "event_id": "c5d6...",
  "type": "cargo.status_changed",
  "occurred_at": "2026-11-10T12:05:00Z",
  "payload": {
    "cargo_id": "1b2c...",
    "old_status": "pending",
    "new_status": "in_transit"
  }
}
```

Publisher confirms are enabled, so the API waits for the broker to acknowledge a publish before reporting success at the event layer. A publish failure does not fail the HTTP request, because the status change is already committed; the failure is logged for retry/operator attention.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `HTTP_ADDR` | `:8080` | HTTP listen address |
| `DATABASE_URL` | `postgres://deus:deus@localhost:5432/deus?sslmode=disable` | Postgres DSN |
| `RABBITMQ_URL` | `amqp://deus_user:deus_pass@localhost:5672/` | RabbitMQ URL |
| `RABBITMQ_EXCHANGE` | `cargo.events` | Topic exchange |
| `RABBITMQ_QUEUE` | `cargo.events.worker` | Consumer queue |
| `RABBITMQ_ROUTING_KEY` | `cargo.status.changed` | Routing key |
| `RUN_MIGRATIONS` | `true` | Apply migrations on API startup |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

The credentials in this repository are for local development only.

## Design decisions

- **Standard library HTTP.** Go 1.22+ `ServeMux` supports method and path patterns, so no third-party router is needed. Middleware stays ordinary `func(http.Handler) http.Handler`.
- **Explicit SQL over an ORM.** The schema and queries are visible, indexes are intentional, and behavior is easy to explain. `pgx` avoids hidden query generation.
- **Interfaces at the consumer.** Repositories and publishers are interfaces declared in `service`; implementations live in infrastructure packages. This keeps tests fast and dependencies one-directional.
- **State machine in the domain.** `CargoStatus.CanTransitionTo` is the single source of truth for allowed transitions. The repository re-checks the expected status inside a transaction to protect against races.
- **Append-only tracking.** Tracking rows are never updated or deleted. Status changes and manual movements add rows, preserving history.
- **Idempotent consumer.** `cargo_events.event_id` is unique and inserts use `ON CONFLICT DO NOTHING`, so duplicate deliveries do not duplicate audit records.
- **Durable queue topology.** Durable exchanges/queues, persistent messages, publisher confirms, manual acknowledgements, prefetch, and a dead-letter queue are configured from the start.
- **Graceful shutdown.** Both binaries use `signal.NotifyContext`; the API drains in-flight requests with `http.Server.Shutdown`, and the consumer stops on context cancellation.
- **Structured logging.** `slog` JSON logs include `request_id`, method, path, status, and duration. Request IDs are returned in the `X-Request-ID` header.

### Known trade-offs and next steps

- Publishing happens after the database commit, so a broker outage can drop an event even though the status change succeeded. The production-grade fix is a transactional outbox with a relay process.
- The consumer requeues storage failures indefinitely. A retry counter and a policy to move exhausted messages to the DLQ would be the next addition.
- Authentication, authorization, rate limiting, and OpenAPI documentation are intentionally out of scope for this challenge.
- Integration tests use a compose-provided database. `testcontainers-go` would make them self-contained.
- `golang-migrate` runs with an advisory lock, so concurrent API replicas are safe, but a dedicated migration job is often preferable in production.

## Module name

The module path is `github.com/example/deus-rabbitmq-example`. Rename it in `go.mod` and update imports if you publish this under a different repository.
