#!/usr/bin/env bash
set -euo pipefail

# Validate compose configuration
docker compose config >/dev/null

# Check required environment variables
if [ -z "${DATABASE_URL:-}" ]; then
  export DATABASE_URL="postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable"
fi

if [ -z "${REDIS_URL:-}" ]; then
  export REDIS_URL="redis://localhost:6379/0"
fi

echo "Starting local development environment..."
docker compose up -d

echo "Waiting for services to be healthy..."
docker compose exec api curl -f http://localhost:8080/health || echo "API health check will be available after startup"

echo "Local environment started successfully!"
echo "  API: http://localhost:8080"
echo "  Web: http://localhost:3000"
echo "  PostgreSQL: localhost:5432"
echo "  Redis: localhost:6379"
