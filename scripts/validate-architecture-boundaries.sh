#!/usr/bin/env bash
set -euo pipefail

echo "=== Architecture boundaries ==="

if [ ! -f db/schema.sql ]; then
  echo "FAIL: db/schema.sql is required as the schema baseline"
  exit 1
fi

if [ ! -d db/migrations ]; then
  echo "FAIL: db/migrations is required as the migration boundary"
  exit 1
fi

if ! find db/migrations -type f -name '*.sql' | grep -q .; then
  echo "FAIL: db/migrations must contain at least one SQL migration"
  exit 1
fi

db_refs=$(rg -n '\*gorm\.DB|gorm\.DB|gorm\.Open|gorm\.Config|gorm\.ErrRecordNotFound' internal --glob '*.go' || true)
if [ -n "$db_refs" ]; then
  invalid_db_refs=$(printf '%s\n' "$db_refs" | rg -v '^(internal/database/|internal/app/)' || true)
  if [ -n "$invalid_db_refs" ]; then
    echo "FAIL: gorm references are only allowed in internal/database and internal/app composition"
    printf '%s\n' "$invalid_db_refs"
    exit 1
  fi
fi

query_refs=$(rg -n '\.(Raw|Exec|Table|Model)\(' internal --glob '*.go' || true)
if [ -n "$query_refs" ]; then
  invalid_query_refs=$(printf '%s\n' "$query_refs" | rg -v '^internal/database/' || true)
  if [ -n "$invalid_query_refs" ]; then
    echo "FAIL: raw/complex DB queries are only allowed behind internal/database repositories"
    printf '%s\n' "$invalid_query_refs"
    exit 1
  fi
fi

env_refs=$(rg -n 'os\.Getenv' internal --glob '*.go' || true)
if [ -n "$env_refs" ]; then
  invalid_env_refs=$(printf '%s\n' "$env_refs" | rg -v '^(internal/config/|internal/app/|internal/database/bootstrap\.go:)' || true)
  if [ -n "$invalid_env_refs" ]; then
    echo "FAIL: environment access must stay in config, app wiring, or database bootstrap escape hatches"
    printf '%s\n' "$invalid_env_refs"
    exit 1
  fi
fi

echo "OK: schema and migration boundaries are present"
echo "OK: gorm references stay in repository/app composition layers"
echo "OK: complex DB queries stay inside internal/database"
echo "OK: environment access stays in approved wiring layers"
