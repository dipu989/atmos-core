#!/usr/bin/env bash
set -euo pipefail

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()   { echo -e "${GREEN}✓${NC} $*"; }
warn() { echo -e "${YELLOW}!${NC} $*"; }
fail() { echo -e "${RED}✗${NC} $*"; exit 1; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo ""
echo "  Atmos Core — bootstrap"
echo "  ──────────────────────────────────────────────"
echo ""

# ── 1. Required tools ─────────────────────────────────────────────────────────
echo "Checking required tools..."

for tool in go docker docker-compose curl; do
  if command -v "$tool" > /dev/null 2>&1; then
    ok "$tool found ($(command -v "$tool"))"
  else
    fail "$tool is required but not installed"
  fi
done

# ── 2. Air (live reload) ──────────────────────────────────────────────────────
if command -v air > /dev/null 2>&1; then
  ok "air found"
else
  warn "air not found — installing..."
  go install github.com/air-verse/air@latest
  ok "air installed"
fi

# ── 3. swag (swagger codegen) ─────────────────────────────────────────────────
if command -v swag > /dev/null 2>&1; then
  ok "swag found"
else
  warn "swag not found — installing..."
  go install github.com/swaggo/swag/cmd/swag@latest
  ok "swag installed"
fi

# ── 4. Environment file ───────────────────────────────────────────────────────
if [[ -f ".env" ]]; then
  ok ".env found"
else
  warn ".env not found — copying from .env.example"
  cp .env.example .env
  warn "Edit .env and set JWT_ACCESS_SECRET and JWT_REFRESH_SECRET before running"
fi

# Validate required vars
source .env 2>/dev/null || true
[[ -z "${JWT_ACCESS_SECRET:-}" || "$JWT_ACCESS_SECRET" == *"change-me"* ]] && \
  warn "JWT_ACCESS_SECRET looks like a placeholder — update .env before running the API"
[[ -z "${JWT_REFRESH_SECRET:-}" || "$JWT_REFRESH_SECRET" == *"change-me"* ]] && \
  warn "JWT_REFRESH_SECRET looks like a placeholder — update .env before running the API"

# ── 5. Start infrastructure ───────────────────────────────────────────────────
echo ""
echo "Starting infrastructure..."
docker compose up -d --remove-orphans postgres || \
  docker compose up -d --force-recreate postgres

echo -n "  Waiting for PostgreSQL"
for i in $(seq 1 30); do
  if docker compose exec -T postgres pg_isready -U atmos -d atmos_dev > /dev/null 2>&1; then
    echo ""
    ok "PostgreSQL is ready"
    break
  fi
  echo -n "."
  sleep 1
  [[ $i -eq 30 ]] && fail "PostgreSQL did not become ready in 30 seconds"
done

# ── 6. Migrations ─────────────────────────────────────────────────────────────
echo ""
echo "Running migrations..."
go run ./cmd/migrate
ok "migrations applied"

# ── 7. Seed ───────────────────────────────────────────────────────────────────
echo ""
echo "Seeding database..."
bash scripts/seed.sh

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo "  ──────────────────────────────────────────────"
ok "Bootstrap complete. Start the API with: make dev"
echo ""
