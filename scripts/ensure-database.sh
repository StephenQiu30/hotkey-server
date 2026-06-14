#!/usr/bin/env bash
# Create PostgreSQL database from DATABASE_URL when it does not exist.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "${ROOT_DIR}/scripts/load-env.sh"

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "FAIL: DATABASE_URL is not set" >&2
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "SKIP: psql not found; database creation requires psql or app auto-init" >&2
  exit 0
fi

DB_NAME="$(python3 - <<'PY'
from urllib.parse import urlparse
import os
u = urlparse(os.environ["DATABASE_URL"])
name = (u.path or "").lstrip("/")
if not name:
    raise SystemExit("FAIL: DATABASE_URL has no database name")
print(name)
PY
)"

if psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -c "SELECT 1" >/dev/null 2>&1; then
  echo "OK: database ${DB_NAME} already exists"
  exit 0
fi

ADMIN_URL="$(python3 - <<'PY'
from urllib.parse import urlparse, urlunparse
import os
u = urlparse(os.environ["DATABASE_URL"])
u = u._replace(path="/postgres")
print(urlunparse(u))
PY
)"

if ! psql "${ADMIN_URL}" -v ON_ERROR_STOP=1 -c "SELECT 1" >/dev/null 2>&1; then
  echo "FAIL: cannot connect to admin database via ${ADMIN_URL}" >&2
  exit 1
fi

EXISTS="$(psql "${ADMIN_URL}" -tAc "SELECT 1 FROM pg_database WHERE datname = '${DB_NAME}'")"
if [[ "${EXISTS}" == "1" ]]; then
  echo "OK: database ${DB_NAME} already exists"
  exit 0
fi

psql "${ADMIN_URL}" -v ON_ERROR_STOP=1 -c "CREATE DATABASE ${DB_NAME}"
echo "OK: created database ${DB_NAME}"
