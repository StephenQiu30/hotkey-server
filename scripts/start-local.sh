#!/usr/bin/env bash
set -euo pipefail

# Validate compose configuration
docker compose config >/dev/null

echo "Starting local development environment..."
docker compose up -d

echo "Waiting for services to be healthy..."
sleep 5
docker compose ps

echo ""
echo "Local environment started successfully!"
echo "  API:      http://localhost:8080"
echo "  Health:   http://localhost:8080/healthz"
echo "  Postgres: localhost:5432"
echo "  Redis:    localhost:6379"
