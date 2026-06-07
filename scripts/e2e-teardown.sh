#!/usr/bin/env bash
# Stop and clean up E2E dependencies.
# Usage: ./scripts/e2e-teardown.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "==> Stopping E2E infrastructure..."
docker compose -f "$ROOT_DIR/docker-compose.e2e.yml" down -v --remove-orphans

echo "==> Cleaning up Git test remote fixture..."
GIT_FIXTURE_DIR="$ROOT_DIR/tests/fixtures/git-remote"
if [ -d "$GIT_FIXTURE_DIR" ]; then
  rm -rf "$GIT_FIXTURE_DIR"
  echo "    Removed $GIT_FIXTURE_DIR"
fi

echo "==> E2E infrastructure stopped and volumes removed."
