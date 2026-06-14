#!/usr/bin/env bash
# Start HotKey (API + worker, single process).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "${ROOT_DIR}/scripts/load-env.sh"

cd "${ROOT_DIR}"
echo "Starting HotKey on ${HTTP_ADDR:-:8080}"
go run ./cmd/hotkey
