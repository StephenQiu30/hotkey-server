#!/usr/bin/env sh
set -eu

dsn=${HOTKEY_TEST_DSN:-}
if test -z "$dsn"; then
  printf '%s\n' 'HOTKEY_TEST_DSN is required' >&2
  exit 1
fi
rows=${HOTKEY_CAPACITY_ROWS:-1000}
case "$rows" in
  ''|*[!0-9]*) printf '%s\n' 'HOTKEY_CAPACITY_ROWS must be a positive integer' >&2; exit 1 ;;
esac
if test "$rows" -le 0; then
  printf '%s\n' 'HOTKEY_CAPACITY_ROWS must be a positive integer' >&2
  exit 1
fi

psql "$dsn" -v ON_ERROR_STOP=1 -v fixture_rows="$rows" <<'SQL' >/dev/null
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', 'capacity-fixture-source', 'https://fixture.invalid/rss')
ON CONFLICT DO NOTHING;

WITH source AS (
  SELECT id FROM source_connections WHERE name = 'capacity-fixture-source' AND deleted_at IS NULL
), generated AS (
  SELECT generate_series(1, :fixture_rows::integer) AS n
)
INSERT INTO contents (
  source_connection_id, external_id, content_type, canonical_url,
  published_at, fetched_at, dedupe_key
)
SELECT
  source.id,
  'capacity-' || generated.n,
  'article',
  'https://fixture.invalid/content/' || generated.n,
  now() - generated.n * interval '1 second',
  now(),
  lpad(generated.n::text, 64, '0')
FROM source CROSS JOIN generated
ON CONFLICT (source_connection_id, external_id) DO NOTHING;
SQL

plan=$(psql "$dsn" -X -A -t -v ON_ERROR_STOP=1 <<'SQL'
EXPLAIN (COSTS OFF)
SELECT id, published_at
FROM contents
WHERE source_connection_id = (
  SELECT id FROM source_connections WHERE name = 'capacity-fixture-source' AND deleted_at IS NULL
)
ORDER BY published_at DESC, id DESC
LIMIT 50;
SQL
)

printf '%s\n' "$plan"
printf '%s\n' "$plan" | grep -Fq 'contents_source_published_idx' || {
  printf '%s\n' 'capacity query did not use contents_source_published_idx' >&2
  exit 1
}
if printf '%s\n' "$plan" | grep -Eiq 'offset|limit[[:space:]]+all'; then
  printf '%s\n' 'capacity query contains an unbounded offset plan' >&2
  exit 1
fi
printf '%s\n' "Capacity fixture and cursor plan verified for ${rows} rows."
