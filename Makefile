SHELL := /bin/sh

COVERAGE_FILE := coverage.out
TEST_DATABASE_URL := postgres://deus:deus@localhost:5432/deus?sslmode=disable
TEST_RABBITMQ_URL := amqp://deus_user:deus_pass@localhost:5672/

.PHONY: help deps run worker migrate-up migrate-down migrate-version test test-integration cover lint openapi-lint fmt docker-run docker-down clean

help:
	@printf '%s\n' \
		'make deps              Start Postgres and RabbitMQ' \
		'make run               Run the API locally' \
		'make worker            Run the event worker locally' \
		'make migrate-up        Apply database migrations' \
		'make migrate-down      Roll back database migrations' \
		'make test              Run unit tests with the race detector' \
		'make test-integration  Run repository integration tests' \
		'make cover             Run tests and print coverage' \
		'make lint              Run go vet (and staticcheck when installed)' \
		'make openapi-lint      Lint openapi.yaml with Redocly' \
		'make fmt               Format Go sources' \
		'make docker-run        Build and start the full stack' \
		'make docker-down       Stop the stack and remove volumes' \
		'make clean             Remove build artifacts'

deps:
	docker compose up -d --wait postgres rabbitmq

run: deps
	go run ./cmd/api

worker: deps
	go run ./cmd/worker

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-version:
	go run ./cmd/migrate version

test:
	go test -race -count=1 ./...

test-integration: deps
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' TEST_RABBITMQ_URL='$(TEST_RABBITMQ_URL)' go test -race -tags=integration -count=1 ./...

cover: deps
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
	docker compose up --build -d

docker-down:
	docker compose down -v

clean:
	rm -rf bin $(COVERAGE_FILE) coverage.html .gocache
