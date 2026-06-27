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

if [ ! -f db/schema.sql ]; then
  echo "FAIL: db/schema.sql is required as the schema baseline"
  exit 1
fi

if [ ! -d db/migrations ]; then
  echo "FAIL: db/migrations is required as the migration boundary"
  exit 1
fi

if [ ! -f db/migrations/000_schema.sql ]; then
  echo "FAIL: db/migrations/000_schema.sql is required as the baseline schema migration"
  exit 1
fi

if ! find db/migrations -type f -name '*.sql' | grep -q .; then
  echo "FAIL: db/migrations must contain at least one SQL migration"
  exit 1
fi

schema_tables=$(search_files '^create table ' db/schema.sql | sed -E 's/^.*create table ([a-z_]+).*/\1/' | sort)
migration_tables=$(find db/migrations -type f -name '*.sql' -print0 | xargs -0 grep -nE '^create table( if not exists)? ' 2>/dev/null | sed -E 's/^.*create table( if not exists)? ([a-z_]+).*/\2/' | sort -u)
missing_tables=$(comm -23 <(printf '%s\n' "$schema_tables") <(printf '%s\n' "$migration_tables") || true)
if [ -n "$missing_tables" ]; then
  echo "FAIL: db/migrations does not cover schema tables"
  printf '%s\n' "$missing_tables"
  exit 1
fi

db_refs=$(search_go '\*gorm\.DB|gorm\.DB|gorm\.Open|gorm\.Config|gorm\.ErrRecordNotFound' internal)
if [ -n "$db_refs" ]; then
  invalid_db_refs=$(printf '%s\n' "$db_refs" | grep -Ev '^(internal/database/|internal/app/)' || true)
  if [ -n "$invalid_db_refs" ]; then
    echo "FAIL: gorm references are only allowed in internal/database and internal/app composition"
    printf '%s\n' "$invalid_db_refs"
    exit 1
  fi
fi

query_refs=$(search_go '\.(Raw|Exec|Table|Model)\(' internal)
if [ -n "$query_refs" ]; then
  invalid_query_refs=$(printf '%s\n' "$query_refs" | grep -Ev '^internal/database/' || true)
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
  invalid_route_json_refs=$(printf '%s\n' "$route_json_refs" | grep -Ev '^internal/platform/http/(errors|response|router)\.go:' || true)
  if [ -n "$invalid_route_json_refs" ]; then
    echo "FAIL: business HTTP routes must use unified responders instead of c.JSON"
    printf '%s\n' "$invalid_route_json_refs"
    exit 1
  fi
fi

echo "OK: schema and migration boundaries are present"
echo "OK: gorm references stay in repository/app composition layers"
echo "OK: complex DB queries stay inside internal/database"
echo "OK: environment access stays in approved wiring layers"
echo "OK: business HTTP routes use unified responders"
