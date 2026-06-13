#!/usr/bin/env bash
# smoke-api.sh — Main-program-level API smoke test.
#
# Builds the server binary, starts it with SMOKE_TEST=1 (bypasses auth),
# and validates that critical endpoints respond with correct status codes
# and non-empty payloads where expected.
#
# Exit 0 = all checks pass; exit 1 = at least one failure.

set -euo pipefail

SMOKE_PORT=18080
BASE="http://127.0.0.1:${SMOKE_PORT}"
BINARY=""
SERVER_PID=""
FAILURES=0

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
assert_status() {
  local url="$1" method="$2" expected="$3" data="${4:-}"
  local status
  if [ -n "$data" ]; then
    status=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "$url" \
      -H 'Content-Type: application/json' -d "$data")
  else
    status=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "$url")
  fi
  if [ "$status" != "$expected" ]; then
    fail "$method $url — expected $expected, got $status"
  else
    pass "$method $url → $status"
  fi
}

# assert_json_field URL METHOD FIELD [DATA]
# Fails if the JSON response does not contain a non-empty, non-null value for FIELD.
assert_json_field() {
  local url="$1" method="$2" field="$3" data="${4:-}"
  local body
  if [ -n "$data" ]; then
    body=$(curl -s -X "$method" "$url" \
      -H 'Content-Type: application/json' -d "$data")
  else
    body=$(curl -s -X "$method" "$url")
  fi
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
go build -o "$BINARY" ./cmd/api
echo "OK: binary built at $BINARY"

echo ""
echo "=== Starting server on :${SMOKE_PORT} ==="
SMOKE_TEST=1 DATABASE_URL="postgres://dummy:dummy@localhost:5432/hotkey?sslmode=disable" \
  JWT_SECRET="smoke-test-secret" \
  HTTP_ADDR=":${SMOKE_PORT}" \
  "$BINARY" api &
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

# --- smoke checks ----------------------------------------------------------

echo ""
echo "=== Smoke: /healthz ==="
assert_status "$BASE/healthz" GET 200

echo ""
echo "=== Smoke: auth/register ==="
assert_status "$BASE/api/v1/auth/register" POST 201 \
  '{"email":"smoke@example.com","password":"Passw0rd!","display_name":"SmokeUser"}'
assert_json_field "$BASE/api/v1/auth/register" POST "id" \
  '{"email":"smoke2@example.com","password":"Passw0rd!","display_name":"SmokeUser2"}'
assert_json_field "$BASE/api/v1/auth/register" POST "email" \
  '{"email":"smoke3@example.com","password":"Passw0rd!","display_name":"SmokeUser3"}'

echo ""
echo "=== Smoke: auth/login ==="
assert_status "$BASE/api/v1/auth/login" POST 200 \
  '{"email":"smoke@example.com","password":"Passw0rd!"}'

echo ""
echo "=== Smoke: monitors (authenticated via SMOKE_TEST bypass) ==="
assert_status "$BASE/api/v1/monitors" GET 200

echo ""
echo "=== Smoke: monitors/{id}/posts ==="
assert_status "$BASE/api/v1/monitors/1/posts" GET 200

echo ""
echo "=== Smoke: monitors/{id}/topics ==="
assert_status "$BASE/api/v1/monitors/1/topics" GET 200

echo ""
echo "=== Smoke: monitors/{id}/trends ==="
assert_status "$BASE/api/v1/monitors/1/trends" GET 200

# --- result -----------------------------------------------------------------

echo ""
if [ "$FAILURES" -gt 0 ]; then
  echo "=== SMOKE FAILED: $FAILURES check(s) failed ==="
  exit 1
else
  echo "=== ALL SMOKE CHECKS PASSED ==="
  exit 0
fi
