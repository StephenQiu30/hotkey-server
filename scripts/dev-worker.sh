#!/usr/bin/env bash
# Run worker locally with .env loaded.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "${ROOT_DIR}/scripts/load-env.sh"

cd "${ROOT_DIR}"
echo "Starting worker (DATABASE_URL=${DATABASE_URL})"
go run ./cmd/api worker
