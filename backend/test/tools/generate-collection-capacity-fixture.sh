#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
sh "$root/test/tools/validate-test-environment.sh"

dsn=${HOTKEY_TEST_DSN:-}

positive_integer() {
  name=$1
  value=$2
  case "$value" in
    ''|*[!0-9]*) printf '%s\n' "$name must be a positive integer" >&2; exit 1 ;;
  esac
  if test "$value" -le 0; then
    printf '%s\n' "$name must be a positive integer" >&2
    exit 1
  fi
}

monitors=${HOTKEY_COLLECTION_CAPACITY_MONITORS:-50}
sources=${HOTKEY_COLLECTION_CAPACITY_SOURCES:-100}
candidates=${HOTKEY_COLLECTION_CAPACITY_CANDIDATES:-50000}
jobs=${HOTKEY_COLLECTION_CAPACITY_JOBS:-20}
positive_integer HOTKEY_COLLECTION_CAPACITY_MONITORS "$monitors"
positive_integer HOTKEY_COLLECTION_CAPACITY_SOURCES "$sources"
positive_integer HOTKEY_COLLECTION_CAPACITY_CANDIDATES "$candidates"
positive_integer HOTKEY_COLLECTION_CAPACITY_JOBS "$jobs"
if test "$sources" -lt 2; then
  printf '%s\n' 'HOTKEY_COLLECTION_CAPACITY_SOURCES must be at least 2' >&2
  exit 1
fi

psql "$dsn" -X -v ON_ERROR_STOP=1 \
  -v fixture_monitors="$monitors" \
  -v fixture_sources="$sources" \
  -v fixture_candidates="$candidates" \
  -v fixture_jobs="$jobs" <<'SQL' >/dev/null
BEGIN;

WITH generated AS (
  SELECT generate_series(1, :fixture_sources::integer) AS ordinal
)
INSERT INTO source_connections (source_type, name, endpoint, config, enabled, health_status)
SELECT
  'rss',
  'collection-capacity-source-' || lpad(ordinal::text, 6, '0'),
  'https://fixture.invalid/rss/' || ordinal,
  '{"allow_body_storage":true}'::jsonb,
  true,
  'healthy'
FROM generated
WHERE NOT EXISTS (
  SELECT 1 FROM source_connections existing
  WHERE existing.name = 'collection-capacity-source-' || lpad(generated.ordinal::text, 6, '0')
    AND existing.deleted_at IS NULL
);

WITH generated AS (
  SELECT generate_series(1, :fixture_monitors::integer) AS ordinal
)
INSERT INTO monitors (name, status)
SELECT 'collection-capacity-monitor-' || lpad(ordinal::text, 6, '0'), 'active'
FROM generated
WHERE NOT EXISTS (
  SELECT 1 FROM monitors existing
  WHERE existing.name = 'collection-capacity-monitor-' || lpad(generated.ordinal::text, 6, '0')
    AND existing.deleted_at IS NULL
);

INSERT INTO monitor_config_versions (
  monitor_id, revision, state, collection_interval_seconds, retention_days
)
SELECT monitor.id, 1, 'draft', 1800, 180
FROM monitors monitor
WHERE monitor.name LIKE 'collection-capacity-monitor-%'
  AND monitor.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM monitor_config_versions existing
    WHERE existing.monitor_id = monitor.id AND existing.revision = 1
  );

WITH numbered_monitors AS (
  SELECT monitor.id AS monitor_id, config.id AS config_id,
         row_number() OVER (ORDER BY monitor.name) AS ordinal
  FROM monitors monitor
  JOIN monitor_config_versions config ON config.monitor_id = monitor.id AND config.revision = 1
  WHERE monitor.name LIKE 'collection-capacity-monitor-%'
    AND monitor.deleted_at IS NULL
    AND config.state = 'draft'
), numbered_sources AS (
  SELECT source.id AS source_id, row_number() OVER (ORDER BY source.name) AS ordinal
  FROM source_connections source
  WHERE source.name LIKE 'collection-capacity-source-%'
    AND source.deleted_at IS NULL
), associations AS (
  SELECT monitor.monitor_id, monitor.config_id, source.source_id, slot
  FROM numbered_monitors monitor
  CROSS JOIN generate_series(0, 1) AS slot
  JOIN numbered_sources source
    ON source.ordinal = ((monitor.ordinal * 2 - 2 + slot) % :fixture_sources::integer) + 1
  WHERE monitor.ordinal <= :fixture_monitors::integer
    AND source.ordinal <= :fixture_sources::integer
)
INSERT INTO monitor_sources (
  config_version_id, source_connection_id, query_override, query_signature, priority, enabled
)
SELECT
  association.config_id,
  association.source_id,
  'capacity topic ' || association.monitor_id,
  md5('collection-capacity-query-' || association.monitor_id) || md5('collection-capacity-query-' || association.monitor_id),
  100,
  true
FROM associations association
ON CONFLICT (config_version_id, source_connection_id) DO NOTHING;

UPDATE monitor_config_versions config
SET state = 'published',
    config_hash = md5('collection-capacity-config-' || config.monitor_id) || md5('collection-capacity-config-' || config.monitor_id),
    published_at = now(),
    updated_at = now()
FROM monitors monitor
WHERE config.monitor_id = monitor.id
  AND config.revision = 1
  AND config.state = 'draft'
  AND monitor.name LIKE 'collection-capacity-monitor-%';

UPDATE monitors monitor
SET published_config_version_id = config.id, updated_at = now()
FROM monitor_config_versions config
WHERE config.monitor_id = monitor.id
  AND config.revision = 1
  AND config.state = 'published'
  AND monitor.name LIKE 'collection-capacity-monitor-%'
  AND monitor.published_config_version_id IS DISTINCT FROM config.id;

INSERT INTO source_checkpoints (monitor_source_id, query_hash, next_poll_at)
SELECT source.id, source.query_signature, now()
FROM monitor_sources source
JOIN monitor_config_versions config ON config.id = source.config_version_id
JOIN monitors monitor ON monitor.id = config.monitor_id
WHERE monitor.name LIKE 'collection-capacity-monitor-%'
ON CONFLICT (monitor_source_id) DO NOTHING;

WITH numbered_sources AS (
  SELECT source.id AS source_id, source.name,
         row_number() OVER (ORDER BY source.name) AS ordinal
  FROM source_connections source
  WHERE source.name LIKE 'collection-capacity-source-%'
    AND source.deleted_at IS NULL
  ORDER BY source.name
  LIMIT :fixture_sources::integer
), run_facts AS (
  SELECT source_id, ordinal,
         (:fixture_candidates::integer / :fixture_sources::integer)
           + CASE WHEN ordinal <= (:fixture_candidates::integer % :fixture_sources::integer) THEN 1 ELSE 0 END AS item_count
  FROM numbered_sources
)
INSERT INTO collection_runs (
  source_connection_id, query_signature, window_start, window_end,
  trigger_type, scheduled_at, started_at, finished_at, status,
  candidate_count, accepted_count, rejected_count, page_count
)
SELECT
  source_id,
  md5('collection-capacity-run-' || source_id) || md5('collection-capacity-run-' || source_id),
  timestamptz '2026-01-01 00:00:00+00',
  timestamptz '2026-01-01 00:30:00+00',
  'schedule', now(), now(), now(), 'succeeded', item_count, item_count, 0, 1
FROM run_facts
ON CONFLICT (source_connection_id, query_signature, window_start, window_end) DO NOTHING;

WITH numbered_sources AS (
  SELECT source.id AS source_id,
         row_number() OVER (ORDER BY source.name) AS ordinal
  FROM source_connections source
  WHERE source.name LIKE 'collection-capacity-source-%'
    AND source.deleted_at IS NULL
  ORDER BY source.name
  LIMIT :fixture_sources::integer
), generated AS (
  SELECT ordinal,
         ((ordinal - 1) % :fixture_sources::integer) + 1 AS source_ordinal,
         ((ordinal - 1) / :fixture_sources::integer) + 1 AS source_item_ordinal
  FROM generate_series(1, :fixture_candidates::integer) AS ordinal
), selected_runs AS (
  SELECT run.id AS run_id, source.source_id
  FROM numbered_sources source
  JOIN collection_runs run
    ON run.source_connection_id = source.source_id
   AND run.query_signature = md5('collection-capacity-run-' || source.source_id) || md5('collection-capacity-run-' || source.source_id)
   AND run.window_start = timestamptz '2026-01-01 00:00:00+00'
   AND run.window_end = timestamptz '2026-01-01 00:30:00+00'
)
INSERT INTO collection_run_items (
  run_id, source_code, external_id, content_type, captured_item_version,
  captured_item, payload_hash, raw_payload_disposition, outcome,
  observed_at, source_connection_id
)
SELECT
  selected.run_id,
  'rss',
  'collection-capacity-item-' || generated.source_item_ordinal,
  'article',
  'source-captured-v1',
  jsonb_build_object('fixture', true, 'ordinal', generated.ordinal),
  md5('collection-capacity-item-' || generated.ordinal) || md5('collection-capacity-item-' || generated.ordinal),
  'captured_item_only',
  'captured',
  now(),
  selected.source_id
FROM generated
JOIN numbered_sources source ON source.ordinal = generated.source_ordinal
JOIN selected_runs selected ON selected.source_id = source.source_id
ON CONFLICT (run_id, external_id) DO NOTHING;

INSERT INTO river_job (kind, args, state, max_attempts, priority, scheduled_at, unique_key)
SELECT
  'collect_source', '{}', 'available', 3, 2, now(),
  convert_to('collection-capacity-job-' || lpad(ordinal::text, 6, '0'), 'UTF8')
FROM generate_series(1, :fixture_jobs::integer) AS ordinal
ON CONFLICT (kind, unique_key) DO NOTHING;

COMMIT;
SQL

facts=$(psql "$dsn" -X -A -t -F '|' -v ON_ERROR_STOP=1 <<'SQL'
SELECT
  (SELECT count(*) FROM monitors WHERE name LIKE 'collection-capacity-monitor-%' AND status='active' AND deleted_at IS NULL),
  (SELECT count(*) FROM source_connections WHERE name LIKE 'collection-capacity-source-%' AND enabled AND deleted_at IS NULL),
  (SELECT count(*) FROM collection_run_items item JOIN source_connections source ON source.id=item.source_connection_id WHERE source.name LIKE 'collection-capacity-source-%'),
  (SELECT count(*) FROM river_job WHERE kind='collect_source' AND convert_from(unique_key, 'UTF8') LIKE 'collection-capacity-job-%');
SQL
)
expected="${monitors}|${sources}|${candidates}|${jobs}"
if test "$facts" != "$expected"; then
  printf '%s\n' "collection capacity fixture facts ${facts}, want ${expected}; use a fresh isolated database when changing scale" >&2
  exit 1
fi

queue_plan=$(psql "$dsn" -X -A -t -v ON_ERROR_STOP=1 <<'SQL'
EXPLAIN (COSTS OFF)
SELECT id
FROM river_job
WHERE state='available' AND scheduled_at <= now()
ORDER BY priority,id
FOR UPDATE SKIP LOCKED
LIMIT 1;
SQL
)
printf '%s\n' "$queue_plan" | grep -Fq 'river_job_fetch_idx' || {
  printf '%s\n' 'collection queue claim did not use river_job_fetch_idx' >&2
  exit 1
}
printf '%s\n' "Collection capacity fixture verified: monitors=${monitors}, sources=${sources}, candidates=${candidates}, jobs=${jobs}."
