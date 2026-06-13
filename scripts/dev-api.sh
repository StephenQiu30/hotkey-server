#!/usr/bin/env bash
# Run API locally with .env loaded (expects Postgres/Redis on localhost).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "${ROOT_DIR}/scripts/load-env.sh"

cd "${ROOT_DIR}"
echo "Starting API on ${HTTP_ADDR:-:8080} (DATABASE_URL=${DATABASE_URL})"
go run ./cmd/api api
