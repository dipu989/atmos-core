BINARY_API     := bin/api
BINARY_MIGRATE := bin/migrate
BINARY_SEED    := bin/seed

GOFLAGS := -ldflags="-s -w"

.PHONY: all build run dev \
        migrate-up migrate-down migrate-dry \
        seed infra-up infra-down reset-db \
        swag lint fmt vet test \
        smoke-test \
        docker-logs docker-db \
        k8s-build k8s-deploy k8s-delete k8s-status k8s-logs \
        clean help

## ── Default ───────────────────────────────────────────────────────────────────

all: build

## ── Build ─────────────────────────────────────────────────────────────────────

build: swag
	@mkdir -p bin
	go build $(GOFLAGS) -o $(BINARY_API)     ./cmd/api
	go build $(GOFLAGS) -o $(BINARY_MIGRATE) ./cmd/migrate
	go build $(GOFLAGS) -o $(BINARY_SEED)    ./cmd/seed
	@echo "✓ build complete → bin/"

## ── Run ───────────────────────────────────────────────────────────────────────

run: build
	./$(BINARY_API)

dev:
	@which air > /dev/null 2>&1 || go install github.com/air-verse/air@latest
	air

## ── Database ──────────────────────────────────────────────────────────────────

migrate-up:
	go run ./cmd/migrate
	@echo "✓ migrations applied"

migrate-down:
	go run ./cmd/migrate --rollback
	@echo "✓ last migration rolled back"

migrate-dry:
	go run ./cmd/migrate --dry-run

seed:
	@bash scripts/seed.sh

reset-db:
	@bash scripts/reset-db.sh

## ── Infrastructure ────────────────────────────────────────────────────────────

infra-up:
	docker compose up -d postgres
	@echo "✓ postgres ready on localhost:5433"

infra-down:
	docker compose down
	@echo "✓ infrastructure stopped"

docker-logs:
	docker compose logs -f api

docker-db:
	docker compose up -d postgres

## ── Quality ───────────────────────────────────────────────────────────────────

fmt:
	gofmt -w .
	@which goimports > /dev/null 2>&1 && goimports -w . || true
	@echo "✓ formatting done"

lint:
	@which golangci-lint > /dev/null 2>&1 || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	golangci-lint run ./...

vet:
	go vet ./...

test:
	go test ./... -v -race -count=1

## ── Swagger ───────────────────────────────────────────────────────────────────

swag:
	@which swag > /dev/null 2>&1 || go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g cmd/api/main.go -o docs --quiet
	@echo "✓ swagger docs → docs/"

## ── Smoke Test ────────────────────────────────────────────────────────────────

smoke-test:
	@bash scripts/smoke-test.sh

## ── Bootstrap ─────────────────────────────────────────────────────────────────

bootstrap:
	@bash scripts/bootstrap.sh

## ── Kubernetes (minikube) ─────────────────────────────────────────────────────

k8s-build:
	eval $$(minikube docker-env) && docker build -t atmos-api:latest .
	@echo "✓ image built inside minikube"

k8s-deploy: k8s-build
	kubectl apply -f k8s/namespace.yaml
	kubectl apply -f k8s/postgres/
	kubectl apply -f k8s/api/
	@echo "✓ manifests applied"

k8s-delete:
	kubectl delete -f k8s/api/      --ignore-not-found
	kubectl delete -f k8s/postgres/ --ignore-not-found

k8s-status:
	kubectl get pods,svc,ingress -n atmos

k8s-logs:
	kubectl logs -n atmos -l app=atmos-api -f

## ── Cleanup ───────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/ tmp/

## ── Help ──────────────────────────────────────────────────────────────────────

help:
	@echo ""
	@echo "  Atmos Core — available targets"
	@echo ""
	@echo "  Development"
	@echo "  ─────────────────────────────────────────────────"
	@echo "  make bootstrap      First-time setup (tools, infra, migrate, seed)"
	@echo "  make dev            Run API with live reload (Air)"
	@echo "  make run            Build and run the API"
	@echo "  make build          Compile all binaries"
	@echo ""
	@echo "  Database"
	@echo "  ─────────────────────────────────────────────────"
	@echo "  make migrate-up     Apply all pending migrations"
	@echo "  make migrate-down   Roll back the last migration"
	@echo "  make migrate-dry    Preview pending migrations"
	@echo "  make seed           Run all seed scripts"
	@echo "  make reset-db       Drop, recreate, migrate, and seed"
	@echo ""
	@echo "  Infrastructure"
	@echo "  ─────────────────────────────────────────────────"
	@echo "  make infra-up       Start PostgreSQL via Docker Compose"
	@echo "  make infra-down     Stop all Docker Compose services"
	@echo "  make docker-logs    Tail API container logs"
	@echo ""
	@echo "  Quality"
	@echo "  ─────────────────────────────────────────────────"
	@echo "  make fmt            Format all Go files"
	@echo "  make lint           Run golangci-lint"
	@echo "  make vet            Run go vet"
	@echo "  make test           Run all tests"
	@echo "  make swag           Regenerate Swagger docs"
	@echo "  make smoke-test     Run curl-based API smoke tests"
	@echo ""
	@echo "  Kubernetes"
	@echo "  ─────────────────────────────────────────────────"
	@echo "  make k8s-build      Build image into minikube"
	@echo "  make k8s-deploy     Apply all K8s manifests"
	@echo "  make k8s-delete     Remove K8s resources"
	@echo "  make k8s-status     Show pods, services, ingress"
	@echo "  make k8s-logs       Tail API pod logs"
	@echo ""
	@echo "  make clean          Remove build artifacts (bin/, tmp/)"
	@echo ""
