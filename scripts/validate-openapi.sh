#!/usr/bin/env bash
set -euo pipefail

before="$(shasum docs/docs.go docs/swagger.json docs/swagger.yaml)"
make openapi >/dev/null
after="$(shasum docs/docs.go docs/swagger.json docs/swagger.yaml)"

if [[ "$before" != "$after" ]]; then
  echo "OpenAPI artifacts were stale; run make openapi and commit the result." >&2
  exit 1
fi

echo "OK: OpenAPI artifacts match source annotations"
