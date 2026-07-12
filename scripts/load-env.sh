#!/usr/bin/env bash
# Load project .env for shell scripts and make targets.
# Environment variables already set (e.g. from CI step env) take precedence
# over values in .env.
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

# Snapshot env vars that callers may have pre-set (e.g. DATABASE_URL in CI).
_PRE_EXISTING_DATABASE_URL="${DATABASE_URL:-}"

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

# Restore pre-existing env vars so CI/step-level overrides take precedence.
if [ -n "$_PRE_EXISTING_DATABASE_URL" ]; then
  export DATABASE_URL="$_PRE_EXISTING_DATABASE_URL"
fi
