#!/usr/bin/env bash
set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
BASE_URL="${BASE_URL:-http://localhost:8080}"
API="${BASE_URL}/api/v1"

# Test user — use a timestamp suffix to avoid conflicts on repeated runs
TS=$(date +%s)
TEST_EMAIL="smoketest+${TS}@atmos.local"
TEST_PASSWORD="SmokeTest1234!"

# ── Colours & helpers ─────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; NC='\033[0m'

PASS=0; FAIL=0

pass() { echo -e "  ${GREEN}PASS${NC}  $*"; ((PASS++)); }
fail() { echo -e "  ${RED}FAIL${NC}  $*"; ((FAIL++)); }
section() { echo -e "\n${BOLD}$*${NC}"; }

# Run curl, capture HTTP status + body
req() {
  local method="$1"; shift
  local url="$1";    shift
  local extra=("$@")

  local tmp; tmp=$(mktemp)
  local status
  status=$(curl -s -o "$tmp" -w "%{http_code}" -X "$method" \
    -H "Content-Type: application/json" \
    "${extra[@]}" \
    "$url")
  BODY=$(cat "$tmp"); rm -f "$tmp"
  HTTP_STATUS="$status"
}

assert_status() {
  local expected="$1"
  local label="$2"
  if [[ "$HTTP_STATUS" == "$expected" ]]; then
    pass "$label (HTTP $HTTP_STATUS)"
  else
    fail "$label — expected HTTP $expected, got $HTTP_STATUS"
    echo "       response: $BODY"
  fi
}

extract() {
  # Naive jq-free JSON field extractor for simple string/number values
  echo "$BODY" | grep -o "\"$1\":\"[^\"]*\"" | head -1 | sed 's/"'"$1"'":"//;s/"//'
}

echo ""
echo -e "${BOLD}  Atmos Core — smoke tests${NC}"
echo "  Target: $BASE_URL"
echo "  ──────────────────────────────────────────────"

# ── Health ────────────────────────────────────────────────────────────────────
section "1. Health"

req GET "${BASE_URL}/health"
assert_status 200 "GET /health"

# ── Auth — Register ───────────────────────────────────────────────────────────
section "2. Auth — Register"

req POST "${API}/auth/register" \
  -d "{\"email\":\"${TEST_EMAIL}\",\"password\":\"${TEST_PASSWORD}\"}"
assert_status 201 "POST /auth/register"

# ── Auth — Login ──────────────────────────────────────────────────────────────
section "3. Auth — Login"

req POST "${API}/auth/login" \
  -d "{\"email\":\"${TEST_EMAIL}\",\"password\":\"${TEST_PASSWORD}\"}"
assert_status 200 "POST /auth/login"

ACCESS_TOKEN=$(echo "$BODY" | grep -o '"access_token":"[^"]*"' | sed 's/"access_token":"//;s/"//')

if [[ -z "$ACCESS_TOKEN" ]]; then
  fail "could not extract access_token from login response"
  echo "  Response: $BODY"
  echo ""
  echo -e "${RED}Cannot continue without a token.${NC}"
  exit 1
fi

AUTH=(-H "Authorization: Bearer ${ACCESS_TOKEN}")

# ── Auth — Duplicate register ─────────────────────────────────────────────────
section "4. Auth — Duplicate register (conflict)"

req POST "${API}/auth/register" \
  -d "{\"email\":\"${TEST_EMAIL}\",\"password\":\"${TEST_PASSWORD}\"}"
assert_status 409 "POST /auth/register duplicate → 409"

# ── Identity ──────────────────────────────────────────────────────────────────
section "5. Identity"

req GET "${API}/users/me" "${AUTH[@]}"
assert_status 200 "GET /users/me"

req PUT "${API}/users/me" "${AUTH[@]}" \
  -d '{"display_name":"Smoke Tester","timezone":"Asia/Kolkata"}'
assert_status 200 "PUT /users/me"

req GET "${API}/users/me/preferences" "${AUTH[@]}"
assert_status 200 "GET /users/me/preferences"

req PUT "${API}/users/me/preferences" "${AUTH[@]}" \
  -d '{"distance_unit":"km"}'
assert_status 200 "PUT /users/me/preferences"

# ── Activity — Ingest ─────────────────────────────────────────────────────────
section "6. Activity — Ingest"

STARTED_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

req POST "${API}/activities" "${AUTH[@]}" \
  -d "{
    \"transport_mode\": \"metro\",
    \"distance_km\": 12.5,
    \"duration_minutes\": 35,
    \"source\": \"manual\",
    \"started_at\": \"${STARTED_AT}\"
  }"
assert_status 201 "POST /activities (metro)"

ACTIVITY_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"//;s/"//')

req POST "${API}/activities" "${AUTH[@]}" \
  -d "{
    \"transport_mode\": \"walking\",
    \"distance_km\": 2.0,
    \"duration_minutes\": 25,
    \"source\": \"manual\",
    \"started_at\": \"${STARTED_AT}\"
  }"
assert_status 201 "POST /activities (walking)"

# ── Activity — Duplicate (idempotency) ───────────────────────────────────────
req POST "${API}/activities" "${AUTH[@]}" \
  -d "{
    \"transport_mode\": \"metro\",
    \"distance_km\": 12.5,
    \"duration_minutes\": 35,
    \"source\": \"manual\",
    \"started_at\": \"${STARTED_AT}\"
  }"
assert_status 409 "POST /activities duplicate → 409"

# ── Activity — List & Get ─────────────────────────────────────────────────────
section "7. Activity — List & Get"

req GET "${API}/activities" "${AUTH[@]}"
assert_status 200 "GET /activities"

if [[ -n "$ACTIVITY_ID" ]]; then
  req GET "${API}/activities/${ACTIVITY_ID}" "${AUTH[@]}"
  assert_status 200 "GET /activities/:id"
fi

req GET "${API}/activities?limit=10&offset=0" "${AUTH[@]}"
assert_status 200 "GET /activities (paginated)"

# ── Timeline ──────────────────────────────────────────────────────────────────
section "8. Timeline"

TODAY=$(date -u +"%Y-%m-%d")
THIS_MONDAY=$(date -u -v-$(date -u +%u)d +"%Y-%m-%d" 2>/dev/null || \
              date -u -d "last monday" +"%Y-%m-%d" 2>/dev/null || \
              echo "$TODAY")
YEAR=$(date -u +"%Y")
MONTH=$(date -u +"%m" | sed 's/^0//')

req GET "${API}/timeline/daily?date=${TODAY}" "${AUTH[@]}"
assert_status 200 "GET /timeline/daily?date=$TODAY"

req GET "${API}/timeline/weekly?week_start=${THIS_MONDAY}" "${AUTH[@]}"
assert_status 200 "GET /timeline/weekly?week_start=$THIS_MONDAY"

req GET "${API}/timeline/monthly?year=${YEAR}&month=${MONTH}" "${AUTH[@]}"
assert_status 200 "GET /timeline/monthly?year=$YEAR&month=$MONTH"

# ── Auth — Logout ─────────────────────────────────────────────────────────────
section "9. Auth — Logout"

req POST "${API}/auth/logout" "${AUTH[@]}"
assert_status 200 "POST /auth/logout"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "  ──────────────────────────────────────────────"
TOTAL=$((PASS + FAIL))
if [[ $FAIL -eq 0 ]]; then
  echo -e "  ${GREEN}${BOLD}All ${TOTAL} checks passed.${NC}"
else
  echo -e "  ${RED}${BOLD}${FAIL}/${TOTAL} checks failed.${NC}"
  exit 1
fi
echo ""
