#!/usr/bin/env bash
# smoke-api.sh — Main-program-level API smoke test.
#
# Builds the server binary, starts it with SMOKE_TEST=1 (bypasses auth),
# and validates that critical endpoints respond with correct status codes
# and non-empty payloads where expected.
#
# Exit 0 = all checks pass; exit 1 = at least one failure.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Load local .env for defaults (sourced line-by-line so existing env vars
# take precedence — CI can still override via exported variables).
# shellcheck disable=SC1091
source "${ROOT_DIR}/scripts/load-env.sh"

SMOKE_PORT=18080
BASE="http://127.0.0.1:${SMOKE_PORT}"
BINARY=""
SERVER_PID=""
FAILURES=0
TOKEN=""

cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [ -n "$BINARY" ] && [ -f "$BINARY" ]; then
    rm -f "$BINARY"
  fi
}
trap cleanup EXIT

# --- helpers ---------------------------------------------------------------

fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}

pass() {
  echo "OK: $1"
}

# assert_status URL METHOD EXPECTED_STATUS [DATA]
# If TOKEN is set, sends the Authorization header automatically.
assert_status() {
  local url="$1" method="$2" expected="$3" data="${4:-}"
  local status
  local curl_args=(-s -o /dev/null -w '%{http_code}' -X "$method")
  if [ -n "$TOKEN" ]; then
    curl_args+=(-H "Authorization: Bearer $TOKEN")
  fi
  if [ -n "$data" ]; then
    curl_args+=(-H 'Content-Type: application/json' -d "$data")
  fi
  status=$(curl "${curl_args[@]}" "$url")
  if [ "$status" != "$expected" ]; then
    fail "$method $url — expected $expected, got $status"
  else
    pass "$method $url → $status"
  fi
}

# assert_json_field URL METHOD FIELD [DATA]
# If TOKEN is set, sends the Authorization header automatically.
assert_json_field() {
  local url="$1" method="$2" field="$3" data="${4:-}"
  local body
  local curl_args=(-s -X "$method")
  if [ -n "$TOKEN" ]; then
    curl_args+=(-H "Authorization: Bearer $TOKEN")
  fi
  if [ -n "$data" ]; then
    curl_args+=(-H 'Content-Type: application/json' -d "$data")
  fi
  body=$(curl "${curl_args[@]}" "$url")
  local val
  val=$(echo "$body" | jq -r ".$field // empty" 2>/dev/null)
  if [ -z "$val" ]; then
    fail "$method $url — field '$field' is empty/null in response: $body"
  else
    pass "$method $url — $field=$val"
  fi
}

# --- build & start server --------------------------------------------------

echo "=== Building server ==="
BINARY=$(mktemp -t hotkey-smoke.XXXXXX)
go build -o "$BINARY" ./cmd/hotkey
echo "OK: binary built at $BINARY"

echo ""
echo "=== Starting server on :${SMOKE_PORT} ==="
SMOKE_TEST=1 \
  DATABASE_URL="${DATABASE_URL:-postgres://dummy:dummy@localhost:5432/hotkey?sslmode=disable}" \
  JWT_SECRET="${JWT_SECRET:-smoke-test-secret}" \
  X_BEARER_TOKEN="${X_BEARER_TOKEN:-smoke-test-token}" \
  HTTP_ADDR=":${SMOKE_PORT}" \
  "$BINARY" &
SERVER_PID=$!

# Wait for server to be ready (up to 10s).
for i in $(seq 1 20); do
  if curl -s -o /dev/null "$BASE/healthz" 2>/dev/null; then
    break
  fi
  sleep 0.5
done

if ! curl -s -o /dev/null "$BASE/healthz" 2>/dev/null; then
  echo "FAIL: server did not start within 10s"
  exit 1
fi
echo "OK: server is up"

# --- Smoke checks / helpers ---

# Unique suffix per run so repeated smoke tests don't conflict on the same
# database without needing cleanup.
_RUN_SUFFIX=$(date +%s)
_smoke_email() { echo "smoke-$1-${_RUN_SUFFIX}@example.com"; }

echo ""
echo "=== Smoke: /healthz ==="
assert_status "$BASE/healthz" GET 200

echo ""
echo "=== Smoke: auth/register ==="
assert_status "$BASE/api/v1/auth/register" POST 201 \
  "{\"email\":\"$(_smoke_email 1)\",\"password\":\"Passw0rd!\",\"display_name\":\"SmokeUser\"}"
assert_json_field "$BASE/api/v1/auth/register" POST "data.id" \
  "{\"email\":\"$(_smoke_email 2)\",\"password\":\"Passw0rd!\",\"display_name\":\"SmokeUser2\"}"
assert_json_field "$BASE/api/v1/auth/register" POST "data.email" \
  "{\"email\":\"$(_smoke_email 3)\",\"password\":\"Passw0rd!\",\"display_name\":\"SmokeUser3\"}"

echo ""
echo "=== Smoke: auth/login ==="
assert_status "$BASE/api/v1/auth/login" POST 200 \
  "{\"email\":\"$(_smoke_email 1)\",\"password\":\"Passw0rd!\"}"

# Extract token from login response for authenticated requests
TOKEN=$(curl -s -X POST "$BASE/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$(_smoke_email 1)\",\"password\":\"Passw0rd!\"}" \
  | jq -r '.data.token // .data.access_token // empty')
if [ -n "$TOKEN" ]; then
  echo "  OK: obtained auth token (${TOKEN:0:20}...)"
else
  fail "POST /api/v1/auth/login — could not extract token from response"
fi

echo ""
echo "=== Smoke: monitors (authenticated with SMOKE_TEST bypass) ==="

# Ensure user_id=1 exists for SMOKE_TEST=1 bypass (which injects user_id=1).
# This is a silent insert — ignore if the user already exists.
psql "${DATABASE_URL}" 2>/dev/null <<'PSQL'
INSERT INTO users (id, email, password_hash, display_name, created_at, updated_at)
VALUES (1, 'smoke-test-bypass@localhost', 'bypass', 'SmokeBypass', now(), now())
ON CONFLICT (id) DO NOTHING;
PSQL

assert_status "$BASE/api/v1/monitors" GET 200

# Create a monitor so we have a valid ID for monitor-scoped endpoints
echo ""
echo "=== Smoke: create monitor ==="
MONITOR_ID=$(curl -s -X POST "$BASE/api/v1/monitors" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"smoke-monitor","query_text":"test query","poll_interval_minutes":30}' \
  | jq -r '.data.id // empty')
if [ -z "$MONITOR_ID" ]; then
  fail "POST /api/v1/monitors — could not create monitor"
else
  pass "POST /api/v1/monitors — created monitor $MONITOR_ID"
fi

echo ""
echo "=== Smoke: monitors/${MONITOR_ID}/posts ==="
assert_status "$BASE/api/v1/monitors/$MONITOR_ID/posts" GET 200

echo ""
echo "=== Smoke: monitors/${MONITOR_ID}/topics ==="
assert_status "$BASE/api/v1/monitors/$MONITOR_ID/topics" GET 200

echo ""
echo "=== Smoke: monitors/${MONITOR_ID}/trends ==="
assert_status "$BASE/api/v1/monitors/$MONITOR_ID/trends" GET 200

# New endpoints require a real database (DB-dependent, skipped in smoke mode).
echo ""
echo "=== Smoke: HotEvent endpoints (DB required — skip) ==="
echo "  SKIP: /api/v1/trending, /api/v1/hot-events require real DB"

# --- result -----------------------------------------------------------------

echo ""
if [ "$FAILURES" -gt 0 ]; then
  echo "=== SMOKE FAILED: $FAILURES check(s) failed ==="
  exit 1
else
  echo "=== ALL SMOKE CHECKS PASSED ==="
  exit 0
fi
