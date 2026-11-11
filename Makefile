SHELL := /bin/sh

COMPOSE_DEV := docker compose -f docker-compose.yml
COMPOSE_TEST := docker compose -f docker-compose.test.yml

COVERAGE_FILE := coverage.out
TEST_DATABASE_URL := postgres://deus:deus@localhost:5433/deus_test?sslmode=disable
TEST_RABBITMQ_URL := amqp://deus_user:deus_pass@localhost:5673/

.PHONY: help deps test-deps test-deps-down run worker seed migrate-up migrate-down migrate-version test test-integration cover lint openapi-lint fmt docker-run docker-down clean

help:
	@printf '%s\n' \
		'make deps              Start Postgres and RabbitMQ' \
		'make run               Run the API locally' \
		'make worker            Run the event worker locally' \
		'make seed              Load idempotent demo data for manual testing' \
		'make migrate-up        Apply database migrations' \
		'make migrate-down      Roll back database migrations' \
		'make migrate-version   Print the current migration version' \
		'make test              Run unit tests with the race detector' \
		'make test-integration  Run repository integration tests (deps removed after)' \
		'make test-deps         Start the isolated integration-test dependencies' \
		'make test-deps-down    Stop integration deps and remove their volume' \
		'make cover             Run tests and print coverage' \
		'make lint              Run go vet (and staticcheck when installed)' \
		'make openapi-lint      Lint openapi.yaml with Redocly' \
		'make fmt               Format Go sources' \
		'make docker-run        Build, start the full stack and load demo data' \
		'make docker-down       Stop the stack and remove volumes' \
		'make clean             Remove build artifacts'

deps:
	$(COMPOSE_DEV) up -d --wait postgres rabbitmq

test-deps:
	$(COMPOSE_TEST) up -d --wait postgres rabbitmq

test-deps-down:
	$(COMPOSE_TEST) down -v

run: deps
	go run ./cmd/api

worker: deps
	go run ./cmd/worker

seed: deps
	go run ./cmd/seed

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-version:
	go run ./cmd/migrate version

test:
	go test -race -count=1 ./...

test-integration:
	@set -e; \
	trap '$(COMPOSE_TEST) down -v' EXIT; \
	$(COMPOSE_TEST) up -d --wait postgres rabbitmq; \
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' TEST_RABBITMQ_URL='$(TEST_RABBITMQ_URL)' go test -race -tags=integration -count=1 ./...

cover:
	@set -e; \
	trap '$(COMPOSE_TEST) down -v' EXIT; \
	$(COMPOSE_TEST) up -d --wait postgres rabbitmq; \
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' TEST_RABBITMQ_URL='$(TEST_RABBITMQ_URL)' go test -race -tags=integration -coverpkg=./internal/...,./migrations -coverprofile=$(COVERAGE_FILE) ./...
	go tool cover -func=$(COVERAGE_FILE)

lint:
	go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; else printf '%s\n' 'staticcheck not installed; skipping'; fi

fmt:
	gofmt -w cmd internal migrations

openapi-lint:
	@if command -v redocly >/dev/null 2>&1; then \
		redocly lint openapi.yaml; \
	elif command -v npx >/dev/null 2>&1; then \
		npx --yes @redocly/cli lint openapi.yaml; \
	else \
		printf '%s\n' 'Redocly CLI not found. Install it with "brew install redocly/cli/redocly" or install Node.js for npx.'; \
		exit 1; \
	fi

docker-run:
	$(COMPOSE_DEV) up --build -d --wait
	$(COMPOSE_DEV) exec -T api /app/seed

docker-down:
	$(COMPOSE_DEV) down -v

clean:
	rm -rf bin $(COVERAGE_FILE) coverage.html .gocache
