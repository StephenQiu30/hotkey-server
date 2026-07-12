#!/usr/bin/env bash
# build.sh — Reassemble db/tables/*.sql into db/schema.sql
#
# Usage: bash db/tables/build.sh
# Run from the project root or anywhere (path-aware).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT="${SCRIPT_DIR}/../schema.sql"

# Concatenate all .sql files in numeric order. The 00_header.sql already
# contains the preamble comment and CREATE EXTENSION.
{
  for f in "${SCRIPT_DIR}"/*.sql; do
    [ -f "$f" ] || continue
    echo ""
    cat "$f"
  done
} > "$OUTPUT"

count=$(find "${SCRIPT_DIR}" -maxdepth 1 -name '*.sql' | wc -l | tr -d ' ')
echo "OK: $(basename "${OUTPUT}") regenerated from ${count} files in db/tables/"
