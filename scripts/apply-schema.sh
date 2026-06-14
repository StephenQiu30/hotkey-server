#!/usr/bin/env bash
# Apply db/schema.sql to the configured PostgreSQL database.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "${ROOT_DIR}/scripts/load-env.sh"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "FAIL: DATABASE_URL is not set" >&2
  exit 1
fi

bash "${ROOT_DIR}/scripts/ensure-database.sh"

schema_ready() {
  psql "${DATABASE_URL}" -tAc "SELECT to_regclass('public.users') IS NOT NULL" 2>/dev/null | grep -q t
}

if command -v psql >/dev/null 2>&1; then
  if schema_ready; then
    echo "OK: schema already applied"
    exit 0
  fi
  echo "Applying schema via local psql..."
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -f "${ROOT_DIR}/db/schema.sql"
  echo "OK: schema applied"
  exit 0
fi

if command -v docker >/dev/null 2>&1 && docker compose ps postgres >/dev/null 2>&1; then
  if docker compose exec -T postgres \
    psql -U "${POSTGRES_USER:-hotkey}" -d "${POSTGRES_DB:-hotkey}" -tAc \
    "SELECT to_regclass('public.users') IS NOT NULL" 2>/dev/null | grep -q t; then
    echo "OK: schema already applied"
    exit 0
  fi
  echo "Applying schema via docker compose postgres..."
  docker compose exec -T postgres \
    psql -U "${POSTGRES_USER:-hotkey}" -d "${POSTGRES_DB:-hotkey}" \
    -v ON_ERROR_STOP=1 < "${ROOT_DIR}/db/schema.sql"
  echo "OK: schema applied"
  exit 0
fi

echo "FAIL: need psql or a running docker compose postgres service" >&2
exit 1
