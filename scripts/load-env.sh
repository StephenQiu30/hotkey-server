#!/usr/bin/env bash
# Load project .env for shell scripts and make targets.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ROOT_DIR}/.env"
EXAMPLE_FILE="${ROOT_DIR}/.env.example"

if [[ ! -f "${ENV_FILE}" ]]; then
  if [[ -f "${EXAMPLE_FILE}" ]]; then
    echo "Creating .env from .env.example..."
    cp "${EXAMPLE_FILE}" "${ENV_FILE}"
  else
    echo "FAIL: missing .env and .env.example" >&2
    exit 1
  fi
fi

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a
