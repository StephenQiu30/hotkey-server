#!/usr/bin/env bash
# validate-swagger.sh — Validate generated Swagger contract artifacts.
#
# Checks:
#   1. docs/swagger.json exists and is valid JSON
#   2. Swagger version is 2.0
#   3. All expected /api/v1/* paths are present
#   4. All expected operationIds are present
#   5. BearerAuth security definition is declared
#   6. Generated docs.go exists
#
# Exit 0 = all checks pass; exit 1 = at least one failure.

set -euo pipefail

SPEC="docs/swagger.json"
DOCS_GO="docs/docs.go"
FAILURES=0

fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }
pass() { echo "OK: $1"; }

# --- 1. File exists and is valid JSON ---
if [ ! -f "$SPEC" ]; then
  echo "FAIL: $SPEC not found (run \`make swagger\` first)"
  exit 1
fi
pass "$SPEC exists"

if ! python3 -c "import json; json.load(open('$SPEC'))" 2>/dev/null; then
  fail "$SPEC is not valid JSON"
  exit 1
fi
pass "$SPEC is valid JSON"

# --- 2. OpenAPI version ---
version=$(python3 -c "import json; print(json.load(open('$SPEC'))['swagger'])")
if [ "$version" != "2.0" ]; then
  fail "expected swagger version 2.0, got $version"
else
  pass "swagger version is 2.0"
fi

# --- 3. Expected /api/v1/* paths ---
expected_paths=(
  "/api/v1/auth/login"
  "/api/v1/auth/register"
  "/api/v1/monitors"
  "/api/v1/monitors/{id}"
  "/api/v1/monitors/{id}/posts"
  "/api/v1/monitors/{id}/topics"
  "/api/v1/monitors/{id}/trends"
  "/api/v1/notifications"
  "/api/v1/notifications/{id}/read"
  "/api/v1/topics/{id}/trends"
)

for p in "${expected_paths[@]}"; do
  if ! python3 -c "import json; d=json.load(open('$SPEC')); assert '$p' in d['paths']" 2>/dev/null; then
    fail "missing path: $p"
  else
    pass "path present: $p"
  fi
done

# --- 4. Expected operationIds ---
expected_ops=(
  "register"
  "login"
  "list-monitors"
  "create-monitor"
  "get-monitor"
  "update-monitor"
  "list-posts"
  "list-topics"
  "get-monitor-trends"
  "get-topic-trends"
  "list-notifications"
  "mark-notification-read"
  "health-check"
)

for op in "${expected_ops[@]}"; do
  if ! python3 -c "
import json
d = json.load(open('$SPEC'))
found = False
for methods in d['paths'].values():
    for k, v in methods.items():
        if k in ('get','post','put','patch','delete','head','options'):
            if isinstance(v, dict) and v.get('operationId') == '$op':
                found = True
assert found
" 2>/dev/null; then
    fail "missing operationId: $op"
  else
    pass "operationId present: $op"
  fi
done

# --- 5. BearerAuth security definition ---
if ! python3 -c "
import json
d = json.load(open('$SPEC'))
assert 'BearerAuth' in d.get('securityDefinitions', {})
" 2>/dev/null; then
  fail "missing securityDefinition 'BearerAuth'"
else
  pass "securityDefinition 'BearerAuth' present"
fi

# --- 6. docs.go generated ---
if [ ! -f "$DOCS_GO" ]; then
  fail "missing generated $DOCS_GO"
else
  pass "$DOCS_GO exists"
fi

# --- result ---
echo ""
if [ "$FAILURES" -gt 0 ]; then
  echo "=== SWAGGER VALIDATION FAILED: $FAILURES check(s) failed ==="
  exit 1
else
  echo "=== ALL SWAGGER CHECKS PASSED ==="
  exit 0
fi
