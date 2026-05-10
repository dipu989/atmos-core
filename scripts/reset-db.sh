#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()   { echo -e "${GREEN}✓${NC} $*"; }
warn() { echo -e "${YELLOW}!${NC} $*"; }
fail() { echo -e "${RED}✗${NC} $*"; exit 1; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DB_USER="${DB_USER:-atmos}"
DB_NAME="${DB_NAME:-atmos_dev}"
CONTAINER="atmos-postgres"

echo ""
echo "  Atmos Core — reset-db"
echo "  ──────────────────────────────────────────────"
warn "This will destroy all data in '$DB_NAME'. Press Ctrl+C to cancel."
sleep 3

# ── 1. Verify container is running ────────────────────────────────────────────
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
  fail "Container '$CONTAINER' is not running. Run: make infra-up"
fi

# ── 2. Terminate active connections ───────────────────────────────────────────
echo ""
echo "Terminating active connections to '$DB_NAME'..."
docker exec "$CONTAINER" psql -U "$DB_USER" -d postgres -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${DB_NAME}' AND pid <> pg_backend_pid();" \
  > /dev/null 2>&1 || true
ok "connections terminated"

# ── 3. Drop database ──────────────────────────────────────────────────────────
echo "Dropping database '$DB_NAME'..."
docker exec "$CONTAINER" psql -U "$DB_USER" -d postgres -c "DROP DATABASE IF EXISTS ${DB_NAME};" > /dev/null
ok "database dropped"

# ── 4. Recreate database ──────────────────────────────────────────────────────
echo "Creating database '$DB_NAME'..."
docker exec "$CONTAINER" psql -U "$DB_USER" -d postgres -c "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};" > /dev/null
ok "database created"

# ── 5. Migrations ─────────────────────────────────────────────────────────────
echo "Running migrations..."
go run ./cmd/migrate
ok "migrations applied"

# ── 6. Seed ───────────────────────────────────────────────────────────────────
echo "Seeding database..."
bash scripts/seed.sh

echo ""
ok "Database reset complete."
echo ""
