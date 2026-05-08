BINARY_API     := bin/api
BINARY_MIGRATE := bin/migrate
BINARY_SEED    := bin/seed

GOFLAGS := -ldflags="-s -w"

.PHONY: all build run dev stop migrate seed swagger lint fmt test clean docker-up docker-down docker-logs help

## ── Build ────────────────────────────────────────────────────────────────────

all: build

build: swagger
	@mkdir -p bin
	go build $(GOFLAGS) -o $(BINARY_API)     ./cmd/api
	go build $(GOFLAGS) -o $(BINARY_MIGRATE) ./cmd/migrate
	go build $(GOFLAGS) -o $(BINARY_SEED)    ./cmd/seed
	@echo "✓ build complete"

## ── Run ──────────────────────────────────────────────────────────────────────

run: build
	./$(BINARY_API)

dev:
	@which air > /dev/null || go install github.com/air-verse/air@latest
	air

## ── Database ─────────────────────────────────────────────────────────────────

migrate:
	go run ./cmd/migrate

migrate-dry:
	go run ./cmd/migrate --dry-run

seed:
	go run ./cmd/seed

## ── Swagger ──────────────────────────────────────────────────────────────────

swagger:
	@which swag > /dev/null || go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g cmd/api/main.go -o docs --quiet
	@echo "✓ swagger docs generated → docs/"

## ── Quality ──────────────────────────────────────────────────────────────────

lint:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w . 2>/dev/null || true

test:
	go test ./... -v -race -count=1

vet:
	go vet ./...

## ── Docker ───────────────────────────────────────────────────────────────────

docker-up:
	docker compose up -d --build
	@echo "✓ services started"

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f api

docker-db:
	docker compose up -d postgres
	@echo "✓ postgres started on :5433"

## ── Cleanup ──────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/ tmp/

## ── Help ─────────────────────────────────────────────────────────────────────

help:
	@echo ""
	@echo "  Atmos backend — available targets"
	@echo ""
	@echo "  make build        Build all binaries (api, migrate, seed)"
	@echo "  make run          Build and run the API server"
	@echo "  make dev          Run with live reload (requires air)"
	@echo "  make migrate      Apply pending database migrations"
	@echo "  make migrate-dry  Show pending migrations without applying"
	@echo "  make seed         Run all database seeders"
	@echo "  make swagger      Generate Swagger docs"
	@echo "  make lint         Run golangci-lint"
	@echo "  make fmt          Format all Go files"
	@echo "  make test         Run all tests"
	@echo "  make docker-up    Build and start all containers"
	@echo "  make docker-down  Stop all containers"
	@echo "  make docker-db    Start only the PostgreSQL container"
	@echo "  make docker-logs  Tail API container logs"
	@echo "  make clean        Remove build artifacts"
	@echo ""
