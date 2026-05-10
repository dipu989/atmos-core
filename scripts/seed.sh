#!/usr/bin/env bash
set -euo pipefail

GREEN='\033[0;32m'; NC='\033[0m'
ok() { echo -e "${GREEN}✓${NC} $*"; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Load .env if present
if [[ -f ".env" ]]; then
  set -o allexport
  source .env
  set +o allexport
fi

echo ""
echo "  Atmos Core — seed"
echo "  ──────────────────────────────────────────────"
echo "Running all seeders (idempotent)..."

go run ./cmd/seed

ok "Seed complete."
echo ""
