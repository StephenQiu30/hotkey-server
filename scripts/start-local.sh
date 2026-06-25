#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

# shellcheck disable=SC1091
source "${ROOT_DIR}/scripts/load-env.sh"

if [[ -z "${JWT_SECRET:-}" ]]; then
  echo "FAIL: JWT_SECRET is required in .env" >&2
  exit 1
fi

# Validate compose configuration (also reads .env for interpolation)
docker compose config >/dev/null

echo "Starting local development environment..."
docker compose up -d postgres

echo "Waiting for PostgreSQL..."
for i in {1..30}; do
  if docker compose exec -T postgres pg_isready -U "${POSTGRES_USER:-hotkey}" >/dev/null 2>&1; then
    break
  fi
  sleep 1
  if [ "$i" -eq 30 ]; then
    echo "FAIL: PostgreSQL health check timed out"
    docker compose ps
    exit 1
  fi
done

echo "Applying database schema..."
bash "${ROOT_DIR}/scripts/apply-schema.sh"

echo "Starting app..."
docker compose up -d app

echo "Waiting for API health check..."
for i in {1..30}; do
  if curl -fsS http://localhost:8080/healthz >/dev/null 2>&1; then
    docker compose ps
    break
  fi
  sleep 1
  if [ "$i" -eq 30 ]; then
    echo "FAIL: API health check timed out"
    docker compose ps
    exit 1
  fi
done

echo ""
echo "Local environment started successfully!"
echo "  API:      http://localhost:8080"
echo "  Health:   http://localhost:8080/healthz"
echo "  Postgres: localhost:5432 (user=${POSTGRES_USER:-hotkey}, db=${POSTGRES_DB:-hotkey})"
echo ""
echo "Useful commands:"
echo "  make dev          # run API + worker locally with .env"
echo "  make schema       # re-apply db/schema.sql"
