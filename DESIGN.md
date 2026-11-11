# Design Document — Deus Logistics API

This document records the main design decisions for the logistics API, the alternatives that were considered, and the trade-offs that remain. It is meant to be read alongside `README.md`: the README explains how to run and use the system, while this document explains why it is built the way it is.

## 1. Purpose and scope

The service manages three related concerns:

1. **Cargoes** — shipments with a lifecycle state.
2. **Vessels** — ships that transport cargoes.
3. **Tracking** — an append-only history of cargo movements.

It also implements the bonus event flow: a cargo status change is published to RabbitMQ and persisted asynchronously by a worker into `cargo_events`.

Out of scope for this challenge: authentication, authorization, rate limiting, soft deletes, and multi-tenancy. The API is documented by a hand-written OpenAPI 3.0.3 specification in `openapi.yaml`.

## 2. System at a glance

```
Client
  │ HTTP/JSON
  ▼
transport/httpapi ──► service ──► domain
                         │
                         ├──► repository/postgres ──► PostgreSQL
                         │
                         └──► messaging/rabbitmq ──► RabbitMQ
                                                        │
                                                        ▼
                                                      worker
                                                        │
                                                        ▼
                                                   cargo_events
```

Dependencies point in one direction:

```
transport/httpapi  ->  service  ->  domain
                           ^
                repository/postgres / messaging/rabbitmq
```

The domain layer knows nothing about HTTP, SQL, or RabbitMQ. The repository and messaging packages implement interfaces declared by the service layer. `cmd/*` is the composition root where concrete implementations are wired together: `api` (HTTP server), `worker` (event consumer), `migrate` (schema CLI), and `seed` (idempotent demo data for manual testing).

## 3. Decision log

| ID | Decision |
|---|---|
| ARCH-1 | Layered packages with a one-way dependency rule |
| ARCH-2 | Interfaces declared where they are consumed |
| HTTP-1 | Standard library `net/http` instead of a web framework |
| DB-1 | `pgx` with explicit SQL instead of an ORM |
| DB-2 | Text status columns with `CHECK` constraints, not PostgreSQL enums |
| DB-3 | UUID primary keys generated in the application |
| DB-4 | Embedded SQL migrations applied with `golang-migrate` |
| TX-1 | Cargo status changes use `SELECT ... FOR UPDATE` and an expected-status check |
| TX-2 | Status update and tracking insert commit in one transaction |
| API-1 | Narrow `PATCH /cargoes/{id}/status` endpoint, not a generic cargo update |
| API-2 | Consistent JSON error envelope with machine-readable codes |
| API-3 | Hand-written OpenAPI 3.0.3 specification in `openapi.yaml` |
| EVT-1 | Publish after the database commit; a publish failure is logged, not returned |
| EVT-2 | Durable topic exchange, durable queue, publisher confirms, manual ack, DLQ |
| EVT-3 | Consumer idempotency through a unique `event_id` and `ON CONFLICT DO NOTHING` |
| OBS-1 | Structured `slog` JSON logs with request IDs |
| TEST-1 | Hand-written fakes for unit tests, real Postgres/RabbitMQ for integration tests |
| DOCKER-1 | One multi-stage image containing API, worker, and migration binaries |
| OPS-1 | Graceful shutdown for HTTP, database pool, publisher, and consumer |

## 4. Architecture decisions

### ARCH-1 — Layered packages with a one-way dependency rule

**Decision.** Organize code into `domain`, `service`, `transport/httpapi`, `repository/postgres`, and `messaging/rabbitmq`. Imports only point inward: transport depends on service, service depends on domain, and infrastructure implements service interfaces.

**Why.** The business rules are testable without HTTP or a database. The repository can change without touching handlers. The HTTP layer can change without touching SQL. The dependency graph stays acyclic and easy to reason about.

**Alternative considered.** Package-by-feature (for example `internal/cargo`, `internal/vessel`) is also idiomatic Go. It works well for larger systems, but this challenge explicitly asks for handlers/services/repositories/models, and the layer-first shape is easier to walk through in a review.

**Consequence.** Some types are mapped between layers (HTTP DTOs to service inputs, domain entities to responses). That is deliberate: it prevents JSON tags and transport concerns from leaking into the domain.

### ARCH-2 — Interfaces declared where they are consumed

**Decision.** `internal/service/ports.go` declares the narrow repository and publisher interfaces that the services need. `internal/repository/postgres` implements them implicitly. HTTP handlers declare small service interfaces in `internal/transport/httpapi/contracts.go`.

**Why.** This follows the Go idiom “accept interfaces, return structs” and avoids a central `interfaces` package that grows into a dumping ground. Tests can substitute fakes without pulling in Postgres or RabbitMQ.

**Alternative considered.** Defining interfaces next to their implementations is common in other languages but tends to create broad interfaces and import cycles in Go. Consumer-side interfaces keep each contract minimal.

**Consequence.** A concrete type may satisfy several small interfaces. That is intentional and cheap because Go interfaces are implicit.

### HTTP-1 — Standard library `net/http`

**Decision.** Use `http.NewServeMux` with Go 1.22+ method and path patterns such as `PATCH /api/v1/cargoes/{id}/status`, plus ordinary `func(http.Handler) http.Handler` middleware.

**Why.** The API surface is small. The standard library provides routing, path variables, timeouts, and testability without a framework. It also demonstrates comfort with modern Go rather than reaching for Gin by default.

**Alternative considered.** Chi is a reasonable middle ground and is still `net/http`-compatible. Gin and Echo bring conveniences but also dependencies, custom context types, and framework-specific error handling. None of those are needed here.

**Consequence.** Middleware composition and JSON decoding are written explicitly. That is a small amount of code and makes behavior easy to trace.

### DB-1 — `pgx` with explicit SQL

**Decision.** Use `jackc/pgx/v5` with `pgxpool` and hand-written SQL in `internal/repository/postgres`.

**Why.** The schema, indexes, locking, and transaction boundaries remain visible. `pgx` is Postgres-native and avoids the hidden query generation that makes ORMs difficult to explain when behavior gets subtle. It also makes `FOR UPDATE`, `ON CONFLICT`, and `count(*) OVER()` straightforward.

**Alternative considered.**
- **GORM** is faster to start and includes migrations, but it hides SQL, makes lock/transaction behavior less obvious, and adds a layer to explain.
- **`sqlc`** generates type-safe code from SQL and is an excellent production choice. It was not used here because it adds a generation step and generated code that must also be understood; hand-written SQL is more direct for a small challenge.
- **`database/sql` + `lib/pq`** works, but `lib/pq` is in maintenance mode and `pgx` is the stronger Postgres driver.

**Consequence.** More scanning code is required. The `scanCargo`, `scanVessel`, and UUID helpers keep that contained.

### DB-2 — Text status columns with `CHECK` constraints

**Decision.** Store statuses as `TEXT` with `CHECK (status IN ('pending', 'in_transit', 'delivered'))`. Map them to the Go type `domain.CargoStatus`.

**Why.** PostgreSQL enum types complicate schema evolution (adding values, ordering, casting). A text column plus a check constraint enforces integrity while remaining easy to migrate and easy to map to a Go string type.

**Alternative considered.** A PostgreSQL `ENUM` gives stronger type identity but requires `ALTER TYPE` migrations and can be awkward with drivers. An unconstrained text column would rely only on application validation and would allow bad data.

**Consequence.** The allowed values are duplicated in three places by design: the Go state machine, the `CHECK` constraint, and (indirectly) the tests. That duplication is intentional defense in depth.

### DB-3 — UUID primary keys generated in the application

**Decision.** Use `uuid.UUID` values generated with `google/uuid` before insert.

**Why.** The application knows the ID before the round trip, which simplifies resource creation and event correlation. UUIDs also avoid leaking record counts and are safe to generate in distributed clients or services.

**Alternative considered.** `BIGSERIAL` is smaller and simpler for indexes, but it requires a database round trip to obtain the ID and exposes sequential counts. Identity columns are also less convenient for events created before persistence.

**Consequence.** UUID indexes are larger than integer indexes. For this workload that is irrelevant.

### DB-4 — Embedded SQL migrations

**Decision.** Keep migrations in `migrations/`, embed them with `//go:embed` in `migrations/embed.go`, and apply them with `golang-migrate`. The API runs them on startup when `RUN_MIGRATIONS=true`; `cmd/migrate` provides `up`, `down`, and `version` commands.

**Why.** The schema is versioned, reviewable, and deployable with the binary. `golang-migrate` uses an advisory lock, so concurrent API replicas do not run the same migration twice. Embedding avoids a separate migration file copy in the container.

**Alternative considered.** GORM auto-migration is convenient but tends to produce implicit schema changes and is a poor fit when explicit indexes and constraints matter. A migration sidecar or init container is cleaner in production but adds a moving part to the challenge setup.

**Consequence.** Running migrations from application startup is acceptable here. In a production deployment with many replicas or strict rollout ordering, a dedicated migration job is preferable.

## 5. Data model

```
vessels 1 ──── * cargoes 1 ──── * tracking
                     │
                     └──── * cargo_events   (written by the consumer)
```

### `vessels`

- `name` is unique; duplicate names return `409 Conflict`.
- `capacity > 0` is enforced by a check constraint.
- `current_location` is mutable and represents the vessel's current position.

### `cargoes`

- `code` is unique; duplicate codes return `409 Conflict`.
- `weight_kg > 0` is enforced by a check constraint.
- `status` is constrained to the three lifecycle values.
- `vessel_id` is `NOT NULL` and references `vessels(id) ON DELETE RESTRICT`.
- Indexes: `vessel_id`, `status`, and `created_at DESC` (the default list order).

**Why `vessel_id` is required.** The challenge says “a cargo belongs to a vessel”. Making the relationship mandatory keeps the invariant simple. A nullable `vessel_id` would model unassigned cargo but would add assignment endpoints, transition rules, and more validation.

### `tracking`

- Append-only. Rows are inserted by status changes and by `POST /cargoes/{id}/tracking`.
- `cargo_id` references `cargoes(id) ON DELETE RESTRICT`.
- Index on `(cargo_id, occurred_at DESC)` supports history reads.
- No updates or deletes are exposed.

**Why append-only.** Auditability is a requirement. Overwriting a location or status would destroy history. Current state lives on `cargoes`; history lives here.

### `cargo_events`

- `event_id` is unique and comes from the producer; this is the idempotency key.
- `cargo_id` references `cargoes(id) ON DELETE RESTRICT`.
- Index on `(cargo_id, occurred_at DESC)` supports audit queries.
- `received_at` records when the consumer stored the event, independent of when it occurred.

**Why a separate table.** Events are an audit and integration concern, not part of the operational cargo model. Keeping them separate allows the consumer to evolve independently and avoids coupling the API's write path to event storage.

## 6. API conventions

### Narrow status endpoint

`PATCH /api/v1/cargoes/{id}/status` accepts `{"status": "...", "location": "..."}`.

**Why not a generic `PATCH /cargoes/{id}`?** A generic update would need pointer fields to distinguish “not provided” from “set to zero”, and it would mix descriptive fields with lifecycle transitions. A dedicated endpoint makes the state machine explicit, validates the transition, and guarantees that a location is recorded.

### Consistent error envelope

```json
{
  "error": {
    "code": "validation_error",
    "message": "request validation failed",
    "fields": [{"field": "code", "message": "is required"}],
    "request_id": "..."
  }
}
```

Status mapping:

| Condition | HTTP status | Error code |
|---|---|---|
| Malformed JSON or invalid path/query parameter | `400` | `bad_request` |
| Semantic validation failure | `422` | `validation_error` |
| Missing resource | `404` | `not_found` |
| Duplicate resource or stale concurrent update | `409` | `conflict` |
| Disallowed lifecycle transition | `409` | `invalid_status_transition` |
| Unexpected failure | `500` | `internal_error` |

The mapping lives in `internal/transport/httpapi/errors.go`. Internal errors are logged with details but never returned to the client.

### Pagination

`GET /cargoes` supports `status`, `vessel_id`, `limit`, and `offset`.

- Default limit: 20.
- Maximum limit: 100.
- Negative offsets are rejected.

The repository uses `count(*) OVER()` so the total is returned in the same query as the page.

### OpenAPI specification

`openapi.yaml` describes every endpoint, request/response schema, status code, and error envelope. It is the import source for Postman, Insomnia, Bruno, and Swagger UI. OpenAPI 3.0.3 was chosen for broad client support.

The specification is hand-written so it stays readable and does not require a code-generation step. It must be updated when routes, DTOs, or status codes change. Validate it with:

```bash
make openapi-lint
```

The target uses a locally installed `redocly` binary when available and falls back to `npx` for fresh clones. Rule overrides live in `redocly.yaml`: the project has no license URL, the only server is intentionally `localhost`, and the list/health operations do not return 4XX responses.

## 7. Key flows

### 7.1 Create a cargo

1. `CargoHandler.Create` decodes JSON with `DisallowUnknownFields` and a 1 MiB body limit.
2. It parses `vessel_id` as a UUID.
3. `CargoService.Create` trims input, validates code/weight/vessel, and checks that the vessel exists.
4. The service creates a `pending` cargo with timestamps in UTC.
5. The repository inserts the row. A unique violation becomes `domain.ErrConflict`; a foreign-key violation becomes `domain.ErrNotFound`.
6. The handler returns `201 Created`, a `Location` header, and the cargo JSON.

### 7.2 Change cargo status

1. The handler parses the ID and decodes `{status, location}`.
2. The service validates the status string and location.
3. The service reads the cargo to determine the current status.
4. `CargoStatus.CanTransitionTo` enforces `pending → in_transit → delivered`.
5. The repository opens a transaction, runs `SELECT status FROM cargoes WHERE id = $1 FOR UPDATE`, and re-checks the expected status.
6. The repository updates the cargo and inserts a tracking row in the same transaction, then commits.
7. The service publishes a `cargo.status_changed` event with publisher confirms.
8. If publishing fails, the error is logged and the HTTP request still succeeds, because the status change is already durable.
9. The handler returns `200 OK` with the updated cargo.

**Why the read-then-write double check.** The service needs the current status to apply the domain rule. The repository needs the lock and re-check to protect against a concurrent request that changed the status between the read and the write. Without the re-check, two concurrent `pending → in_transit` requests could both pass the domain check; the second would then overwrite the first instead of returning `409 Conflict`.

### 7.3 Consume a status-change event

1. The worker connects to RabbitMQ and declares the topology idempotently.
2. It consumes with manual acknowledgements and prefetch 10.
3. Each message is decoded into `EventEnvelope`.
4. Unknown event types are acknowledged and ignored.
5. Malformed payloads are negatively acknowledged without requeue, which routes them to the dead-letter queue.
6. Valid events are inserted into `cargo_events` with `ON CONFLICT (event_id) DO NOTHING`.
7. A storage failure is negatively acknowledged with requeue so the event can be retried.
8. Successful inserts are acknowledged.

### 7.4 Read tracking history

1. The handler parses the cargo ID.
2. `TrackingService.List` verifies that the cargo exists, so an unknown cargo returns `404` instead of an empty list.
3. The repository returns entries ordered by `occurred_at DESC, created_at DESC`.
4. The handler returns `{items, total}`.

## 8. Event-driven design

### EVT-1 — Publish after commit

**Decision.** The status update commits first, then the service publishes to RabbitMQ. A publish failure is logged but does not fail the HTTP request.

**Why.** The database is the source of truth for cargo state. Returning an error after a successful commit would tell the client the change failed when it did not. The request outcome should reflect the durable state change.

**Trade-off.** This is the classic dual-write problem: if the broker is unavailable after commit, the event can be lost. The production-grade solution is a transactional outbox:

1. Insert an `outbox` row in the same transaction as the cargo update.
2. A relay process publishes outbox rows and marks them sent.
3. Consumers remain idempotent.

The outbox was intentionally left out to keep the challenge scope reasonable. It is documented here as the next reliability step.

### EVT-2 — Durable topology and delivery semantics

- Exchange: `cargo.events` (topic, durable).
- Routing key: `cargo.status.changed`.
- Queue: `cargo.events.worker` (durable).
- Dead-letter exchange: `cargo.events.dlx`.
- Dead-letter queue: `cargo.events.dlq`.
- Messages are persistent and carry `message_id = event_id`.
- Publisher confirms are enabled; the API waits for the broker acknowledgement.
- Consumer prefetch is 10; acknowledgements are manual.

**Why.** Durable topology prevents message loss on broker restart. Confirms catch broker rejection. Manual acknowledgements ensure an event is not removed before it is stored. Prefetch bounds memory and in-flight work.

**Alternative considered.** Fire-and-forget publishing is simpler but silently loses messages on failure. Auto-ack is faster but can drop messages if the worker crashes mid-processing.

### EVT-3 — Idempotent consumer

**Decision.** `cargo_events.event_id` is unique, and inserts use `ON CONFLICT (event_id) DO NOTHING`.

**Why.** RabbitMQ provides at-least-once delivery. A worker can receive the same message more than once after a crash, reconnect, or requeue. The unique key makes redelivery harmless without requiring distributed locks.

**Consequence.** The consumer must generate a stable `event_id` at the producer. The envelope carries it, and the publisher also sets it as the AMQP `message_id`.

## 9. Error handling, logging, and configuration

### Error taxonomy

Domain errors are sentinels in `internal/domain/errors.go`:

- `ErrNotFound`
- `ErrConflict`
- `ErrInvalidTransition`
- `ErrValidation`

Errors are wrapped with `%w` so callers can use `errors.Is` and `errors.As`. The HTTP layer is the only place that knows about status codes. This keeps the service and repository independent of HTTP.

`domain.ValidationError` groups field-level failures and unwraps to `ErrValidation`. The transport layer maps it to `422` and includes a `fields` array.

### Structured logging

- `log/slog` with a JSON handler in `internal/platform/logging`.
- Request middleware logs method, path, status, duration, request ID, and remote address.
- Request IDs come from `X-Request-ID` or are generated, then returned in the response header and error envelope.
- Panics are recovered by middleware, logged, and returned as `500 internal_error`.
- The worker logs event IDs and cargo IDs on success and failure.

**Why `slog` instead of Zap or Zerolog.** It is structured, context-aware, and in the standard library. Zap and Zerolog are excellent and faster, but adding a dependency is unnecessary for this throughput.

### Configuration

Configuration is read from environment variables into a typed struct in `internal/config`. Defaults target the local compose stack, so `make run` works after `make deps`. Invalid values fail fast at startup.

## 10. Testing strategy

### Unit tests

- Domain tests cover the status state machine and validation errors.
- Service tests use hand-written fakes for repositories and the publisher.
- Handler tests exercise the real router and middleware with fake services.
- Consumer tests use a fake `amqp.Acknowledger` and a fake event store to verify ack, Nack, requeue, and dead-letter behavior.

**Why hand-written fakes instead of generated mocks.** The interfaces are small. Fakes make test intent obvious and avoid a code-generation step. Mockery or gomock would be reasonable if the interfaces grew.

### Integration tests

- Gated by the `integration` build tag.
- Require `TEST_DATABASE_URL` and `TEST_RABBITMQ_URL`.
- Repository tests run migrations and truncate tables between tests.
- RabbitMQ tests use unique exchange/queue names and perform a real publish/consume round-trip.
- The tests verify constraints, transactions, `ErrConflict` on stale updates, tracking history, and event idempotency.

**Why not testcontainers.** `docker-compose` is explicitly requested by the challenge and gives a reproducible environment. `testcontainers-go` would make the tests self-contained and is a good next step, at the cost of a heavier dependency and container startup time.

### Coverage

`make cover` runs unit and integration tests with `-coverpkg=./internal/...,./migrations`, which measures the application packages rather than `cmd/` wiring. The measured total is above 80%.

## 11. Container and runtime design

### DOCKER-1 — One image, three binaries

The `Dockerfile` builds `api`, `worker`, and `migrate` into one image. Compose runs the API with the default command and the worker with `command: ["/app/worker"]`.

**Why.** A single build avoids duplicated dependency downloads and keeps the images consistent. Separate binaries give independent process lifecycles and scaling.

**Alternative considered.** Separate images per binary are cleaner for independent releases. One image is simpler for a challenge and still preserves process separation.

The runtime stage is Alpine with a non-root user, `ca-certificates`, and `wget` for healthchecks. The build is static (`CGO_ENABLED=0`), so it does not depend on libc compatibility in the runtime image.

### Compose topology

- `postgres` with a `pg_isready` healthcheck.
- `rabbitmq` with `rabbitmq-diagnostics ping`.
- `api` waits for Postgres and RabbitMQ to be healthy, then applies migrations and serves traffic.
- `worker` waits for Postgres and RabbitMQ and consumes events.

**Why healthchecks.** `depends_on` alone only waits for container start, not for the service to be ready. Healthchecks avoid flaky startup races.

### OPS-1 — Graceful shutdown

- `signal.NotifyContext` handles `SIGINT` and `SIGTERM`.
- The API calls `http.Server.Shutdown` with a timeout, then closes the publisher and database pool.
- The consumer uses a plain `Consume` plus a `select` on `ctx.Done()`; the deferred channel/connection closes run after the loop exits. `ConsumeWithContext` is deliberately avoided because its internal `Channel.Cancel` goroutine races with the deferred `Channel.Close` for the channel's synchronous replies, which can hang shutdown.
- The worker exits cleanly after the consumer returns.

**Why.** In-flight HTTP requests should finish, and the consumer should stop taking new work before the process exits. This avoids dropped requests and unacknowledged in-flight deliveries.

## 12. Failure modes and mitigations

| Failure | Behavior | Mitigation |
|---|---|---|
| Invalid input | `400` or `422` with field details | Validation in service and handlers |
| Missing cargo/vessel | `404` | Repository maps no rows to `ErrNotFound` |
| Duplicate cargo code or vessel name | `409` | Unique constraints mapped to `ErrConflict` |
| Invalid status transition | `409` | Domain state machine |
| Concurrent status updates | One wins, the other gets `409` | `FOR UPDATE` + expected-status check |
| Database unavailable | Readiness fails; requests return `500`; consumer requeues | Healthchecks, pool ping, retry |
| RabbitMQ unavailable at startup | API fails fast | Compose healthcheck ordering |
| RabbitMQ unavailable after startup | Status change commits, publish is logged | Outbox is the next step |
| Duplicate event delivery | No duplicate row | Unique `event_id` + `ON CONFLICT` |
| Malformed event | Dead-lettered | `Nack(false, false)` |
| Persistence failure while consuming | Requeued | `Nack(false, true)` |
| Panic in a handler | `500`, logged | Recovery middleware |
| Shutdown while serving | In-flight requests drain | `http.Server.Shutdown` |

## 13. Trade-offs and future work

1. **Transactional outbox.** The largest reliability gap. Implement an `outbox` table written in the same transaction as the cargo update, plus a relay that publishes and marks rows sent.
2. **Consumer retry policy.** Requeuing forever can hot-loop when the database is down. Add a retry counter or delayed retry queue, then move exhausted messages to the DLQ.
3. **Authentication and authorization.** Not required by the challenge; a real deployment needs it.
4. **OpenAPI maintenance.** `openapi.yaml` is hand-written. Generate it from code or validate it in CI as the API grows.
5. **Observability.** Add OpenTelemetry traces and Prometheus metrics for request latency, publish failures, consumer lag, and DLQ depth.
6. **Pagination cursor.** Offset pagination is fine for a challenge; keyset pagination scales better for large tables.
7. **Soft deletes.** Not implemented. If cargo deletion is added, audit requirements suggest soft deletes or an immutable archive rather than physical deletion.
8. **Testcontainers.** Replace the compose dependency in integration tests with `testcontainers-go` for self-contained CI.
9. **Separate migration job.** Prefer a one-shot migration service in production instead of running migrations from API startup.
10. **Rate limiting and request size limits.** A 1 MiB body limit exists; rate limiting is out of scope.

## 14. How to explain this codebase

A concise walkthrough that follows the actual dependency flow:

1. **Start at `cmd/api/main.go`.** Configuration is loaded, migrations run, the pool and publisher are created, repositories and services are wired, the router is built, and the server starts with graceful shutdown.
2. **Follow a write request.** `internal/transport/httpapi/cargo_handler.go` decodes and validates transport input, then calls `internal/service/cargo.go`.
3. **Explain the business rules.** The service validates fields, checks the vessel, enforces the state machine, and creates the tracking entry.
4. **Explain persistence.** `internal/repository/postgres/cargo.go` runs explicit SQL. Status changes use a transaction, `FOR UPDATE`, and an expected-status re-check.
5. **Explain errors.** `internal/transport/httpapi/errors.go` is the single mapping from domain errors to HTTP status codes and the JSON envelope.
6. **Explain the event path.** The service publishes after commit; `internal/messaging/rabbitmq/publisher.go` uses confirms; `consumer.go` uses manual ack, prefetch, DLQ, and idempotent inserts.
7. **Explain testing.** Unit tests use fakes; integration tests use real Postgres and RabbitMQ behind the `integration` build tag.
8. **Be explicit about trade-offs.** Publishing after commit can lose an event; the outbox pattern is the fix. Requeue-on-failure needs a retry limit. Migrations at startup are convenient but a separate job is better in production.

### Likely interview questions

**Why not GORM?**
It hides SQL and transaction/locking behavior. This challenge values database modeling and explainability, so explicit SQL with `pgx` is a better fit. `sqlc` would also be a strong choice if type-safe generated queries were preferred.

**How do you prevent lost updates?**
The repository locks the cargo row with `SELECT ... FOR UPDATE`, compares the expected status with the current one, and only then updates. A concurrent request that lost the race gets `409 Conflict`.

**What happens if RabbitMQ is down?**
At startup the API fails fast, and compose healthchecks prevent the API from starting before the broker is ready. If the broker goes down later, the status change still commits and the publish failure is logged. A transactional outbox would make delivery reliable.

**How do you avoid duplicate events?**
The producer assigns a stable `event_id`. `cargo_events.event_id` is unique, and the consumer inserts with `ON CONFLICT (event_id) DO NOTHING`. RabbitMQ's at-least-once delivery becomes effectively once for storage.

**Why a separate worker?**
It isolates consumer failures and scaling from the API process. The API can keep serving reads and writes while the consumer reconnects.

**Why is tracking append-only?**
Auditability. Current cargo state is on `cargoes`; history is immutable tracking rows. Status changes and manual movements both append.

**Why standard library HTTP?**
Go 1.22 routing covers the API surface, and middleware is plain `net/http`. Fewer dependencies means less to explain and fewer framework-specific behaviors.

**How are migrations applied?**
They are embedded in the binary with `go:embed` and applied by `golang-migrate` using an advisory lock. `cmd/migrate` provides explicit `up`, `down`, and `version` commands.

**How do you test the repository layer?**
Integration tests run against a real Postgres, apply migrations, truncate between tests, and assert constraints, transactions, and conflict behavior. They are gated by the `integration` build tag.

**What would you change for production?**
Add a transactional outbox, a retry/DLQ policy with counters, authentication, metrics/tracing, a dedicated migration job, and testcontainers-based integration tests.

## 15. Configuration reference

| Variable | Default | Used by |
|---|---|---|
| `HTTP_ADDR` | `:8080` | API |
| `DATABASE_URL` | `postgres://deus:deus@localhost:5432/deus?sslmode=disable` | API, worker, migrate |
| `RABBITMQ_URL` | `amqp://deus_user:deus_pass@localhost:5672/` | API, worker |
| `RABBITMQ_EXCHANGE` | `cargo.events` | API, worker |
| `RABBITMQ_QUEUE` | `cargo.events.worker` | API, worker |
| `RABBITMQ_ROUTING_KEY` | `cargo.status.changed` | API, worker |
| `RUN_MIGRATIONS` | `true` | API |
| `LOG_LEVEL` | `info` | API, worker |

The credentials in the repository are for local development only.
