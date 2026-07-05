#!/usr/bin/env bash
set -euo pipefail

echo "=== Architecture boundaries ==="

search_files() {
  local pattern=$1
  shift

  if command -v rg >/dev/null 2>&1; then
    rg -n "$pattern" "$@" || true
    return
  fi

  grep -RInE "$pattern" "$@" 2>/dev/null || true
}

search_go() {
  local pattern=$1
  local dir=$2

  if command -v rg >/dev/null 2>&1; then
    rg -n "$pattern" "$dir" --glob '*.go' || true
    return
  fi

  find "$dir" -type f -name '*.go' -print0 | xargs -0 grep -nE "$pattern" 2>/dev/null || true
}

db_refs=$(search_go '\*gorm\.DB|gorm\.DB|gorm\.Open|gorm\.Config|gorm\.ErrRecordNotFound' internal)
if [ -n "$db_refs" ]; then
  invalid_db_refs=$(printf '%s\n' "$db_refs" | grep -Ev '^(internal/database/|internal/app/|internal/collector/|internal/aggregator/|internal/cleanup/)' || true)
  if [ -n "$invalid_db_refs" ]; then
    echo "FAIL: gorm references are only allowed in internal/database and internal/app composition"
    printf '%s\n' "$invalid_db_refs"
    exit 1
  fi
fi

query_refs=$(search_go '\.(Raw|Exec|Table|Model)\(' internal)
if [ -n "$query_refs" ]; then
  invalid_query_refs=$(printf '%s\n' "$query_refs" | grep -Ev '^(internal/database/|internal/collector/|internal/cleanup/)' || true)
  if [ -n "$invalid_query_refs" ]; then
    echo "FAIL: raw/complex DB queries are only allowed behind internal/database repositories"
    printf '%s\n' "$invalid_query_refs"
    exit 1
  fi
fi

env_refs=$(search_go 'os\.Getenv' internal)
if [ -n "$env_refs" ]; then
  invalid_env_refs=$(printf '%s\n' "$env_refs" | grep -Ev '^(internal/config/|internal/app/|internal/database/bootstrap\.go:)' || true)
  if [ -n "$invalid_env_refs" ]; then
    echo "FAIL: environment access must stay in config, app wiring, or database bootstrap escape hatches"
    printf '%s\n' "$invalid_env_refs"
    exit 1
  fi
fi

route_json_refs=$(search_go 'c\.JSON\(' internal/platform/http)
if [ -n "$route_json_refs" ]; then
  invalid_route_json_refs=$(printf '%s\n' "$route_json_refs" | grep -Ev '^internal/platform/http/(errors|response|router|trending)\.go:' || true)
  if [ -n "$invalid_route_json_refs" ]; then
    echo "FAIL: business HTTP routes must use unified responders instead of c.JSON"
    printf '%s\n' "$invalid_route_json_refs"
    exit 1
  fi
fi

echo "OK: gorm references stay in repository/app composition layers"
echo "OK: complex DB queries stay inside internal/database"
echo "OK: environment access stays in approved wiring layers"
echo "OK: business HTTP routes use unified responders"
