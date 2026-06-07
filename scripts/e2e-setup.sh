#!/usr/bin/env bash
# Start E2E dependencies and wait for health checks.
# Usage: ./scripts/e2e-setup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "==> Starting E2E infrastructure..."
docker compose -f "$ROOT_DIR/docker-compose.e2e.yml" up -d --wait

echo "==> Creating Git test remote fixture..."
GIT_FIXTURE_DIR="$ROOT_DIR/tests/fixtures/git-remote"
if [ ! -d "$GIT_FIXTURE_DIR" ]; then
  git init --bare "$GIT_FIXTURE_DIR" >/dev/null 2>&1
  echo "    Created bare repo at $GIT_FIXTURE_DIR"
else
  echo "    Git fixture already exists at $GIT_FIXTURE_DIR"
fi

echo "==> E2E infrastructure is healthy."
echo "    PostgreSQL: 127.0.0.1:15432"
echo "    Redis:      127.0.0.1:16379"
echo "    MinIO:      127.0.0.1:19000 (API) / 19001 (Console)"
echo "    Git remote:  $GIT_FIXTURE_DIR"
