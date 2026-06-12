#!/usr/bin/env bash
set -euo pipefail

# Validate compose configuration
docker compose config >/dev/null

echo "Starting local development environment..."
docker compose up -d

echo "Waiting for services to be healthy..."
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
echo "  Postgres: localhost:5432"
echo "  Redis:    localhost:6379"
