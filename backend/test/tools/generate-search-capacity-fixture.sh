#!/usr/bin/env sh
set -eu

dsn=${HOTKEY_TEST_DSN:-}
if test -z "$dsn"; then
  printf '%s\n' 'HOTKEY_TEST_DSN is required' >&2
  exit 1
fi

rows=${HOTKEY_SEARCH_CAPACITY_ROWS_PER_RESOURCE:-1000}
case "$rows" in
  ''|*[!0-9]*) printf '%s\n' 'HOTKEY_SEARCH_CAPACITY_ROWS_PER_RESOURCE must be a positive integer' >&2; exit 1 ;;
esac
if test "$rows" -le 0 || test "$rows" -gt 100000; then
  printf '%s\n' 'HOTKEY_SEARCH_CAPACITY_ROWS_PER_RESOURCE must be between 1 and 100000' >&2
  exit 1
fi

psql "$dsn" -X -v ON_ERROR_STOP=1 -v fixture_rows="$rows" <<'SQL' >/dev/null
BEGIN;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM source_connections WHERE name='search-capacity-source')
     OR EXISTS (SELECT 1 FROM micro_events WHERE clustering_profile_version='search-capacity-v1')
     OR EXISTS (SELECT 1 FROM knowledge_documents WHERE vault_path LIKE 'events/search-capacity-%') THEN
    RAISE EXCEPTION 'search capacity facts already exist; use a fresh isolated database';
  END IF;
END;
$$;

INSERT INTO users (email,password_hash,display_name,role,status)
VALUES ('search-capacity-admin@fixture.invalid','fixture-not-a-credential','Search capacity fixture administrator','admin','active');

INSERT INTO source_connections (source_type,name,endpoint,config,enabled,health_status)
VALUES ('rss','search-capacity-source','https://fixture.invalid/search-capacity','{"allow_body_storage":true}'::jsonb,true,'healthy');

WITH fixture AS (
  SELECT source.id AS source_id,user_account.id AS actor_id
  FROM source_connections AS source
  CROSS JOIN users AS user_account
  WHERE source.name='search-capacity-source'
    AND user_account.email='search-capacity-admin@fixture.invalid'
)
INSERT INTO source_rights_policies (
  recorded_by_user_id,approved_by_user_id,idempotency_key,command_fingerprint,
  source_connection_id,scope_type,scope_subject,policy_revision,priority,
  basis_summary,policy_hash,effective_at
)
SELECT actor_id,actor_id,'search.capacity.policy.v1',repeat('1',64),source_id,
       'source_endpoint','https://fixture.invalid/search-capacity',1,200,
       'Synthetic search capacity fixture authorization',repeat('2',64),now()-interval '1 day'
FROM fixture;

WITH fixture AS (
  SELECT source.id AS source_id,policy.id AS policy_id,policy.version,policy.policy_hash,policy.recorded_by_user_id
  FROM source_connections AS source
  JOIN source_rights_policies AS policy ON policy.source_connection_id=source.id
  WHERE source.name='search-capacity-source' AND policy.idempotency_key='search.capacity.policy.v1'
)
INSERT INTO source_rights_decision_batches (
  source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
  recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
)
SELECT source_id,policy_id,version,'source_endpoint',source_id::text,policy_hash,
       recorded_by_user_id,'search.capacity.rights.v1',repeat('3',64),3
FROM fixture;

WITH fixture AS (
  SELECT batch.id AS batch_id,batch.source_connection_id,policy.id AS policy_id,
         policy.policy_revision,policy.scope_type,policy.scope_subject,policy.priority,
         policy.basis_summary,policy.policy_hash,policy.effective_at
  FROM source_rights_decision_batches AS batch
  JOIN source_rights_policies AS policy ON policy.id=batch.policy_id
  WHERE batch.idempotency_key='search.capacity.rights.v1'
), actions(action,retention_days) AS (
  VALUES ('display_private'::varchar,NULL::integer),('store_derived'::varchar,NULL::integer),('retain'::varchar,365)
)
INSERT INTO source_rights_decisions (
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,reason_codes,
  evaluator,evaluated_at,effective_from,retention_days
)
SELECT fixture.batch_id,fixture.source_connection_id,fixture.policy_id,fixture.policy_revision,fixture.scope_type,
       fixture.scope_subject,fixture.priority,fixture.basis_summary,'source_endpoint',fixture.source_connection_id::text,
       fixture.policy_hash,actions.action,'allow',ARRAY['synthetic_capacity_fixture'],
       'search-capacity-fixture',now()-interval '1 hour',fixture.effective_at,actions.retention_days
FROM fixture CROSS JOIN actions;

WITH fixture AS (
  SELECT id AS source_id FROM source_connections WHERE name='search-capacity-source'
), generated AS (
  SELECT generate_series(1,:fixture_rows::integer) AS ordinal
)
INSERT INTO source_observations (
  source_connection_id,external_id,upstream_identity,source_code,content_type,title,language,
  canonical_url,body_origin,completeness,published_at,discovered_at,captured_at
)
SELECT fixture.source_id,'search-capacity-'||generated.ordinal,
       md5('search-capacity-upstream-'||generated.ordinal)||md5('search-capacity-upstream-'||generated.ordinal),
       'rss','article','capacitytoken 芯片 release '||generated.ordinal,'zh-CN',
       'https://fixture.invalid/search-capacity/'||generated.ordinal,'feed_content','full',
       now()-generated.ordinal*interval '1 second',now()-generated.ordinal*interval '1 second',
       now()-generated.ordinal*interval '1 second'
FROM fixture CROSS JOIN generated;

INSERT INTO documents (source_connection_id,document_key,external_work_id)
SELECT observation.source_connection_id,
       md5('search-capacity-document-'||observation.external_id)||md5('search-capacity-document-'||observation.external_id),
       observation.external_id
FROM source_observations AS observation
JOIN source_connections AS source ON source.id=observation.source_connection_id
WHERE source.name='search-capacity-source';

INSERT INTO document_versions (
  document_id,source_observation_id,revision_no,version_key,body_origin,completeness,word_count,language,
  content_sha256,extractor_version,extractor_profile_version,extractor_profile_sha256,lifecycle_state,captured_at
)
SELECT document.id,observation.id,1,
       md5('search-capacity-version-'||document.id)||md5('search-capacity-version-'||document.id),
       observation.body_origin,observation.completeness,24,observation.language,
       md5('search-capacity-content-'||document.id)||md5('search-capacity-content-'||document.id),
       'search-capacity-v1','search-capacity-v1',repeat('4',64),'derive_pending',observation.captured_at
FROM documents AS document
JOIN source_connections AS source ON source.id=document.source_connection_id AND source.name='search-capacity-source'
JOIN source_observations AS observation
  ON observation.source_connection_id=document.source_connection_id AND observation.external_id=document.external_work_id;

WITH decisions AS (
  SELECT source.id AS source_id,
         max(decision.id) FILTER (WHERE decision.action='store_derived') AS store_id,
         max(decision.id) FILTER (WHERE decision.action='retain') AS retain_id
  FROM source_connections AS source
  JOIN source_rights_decisions AS decision ON decision.source_connection_id=source.id
  WHERE source.name='search-capacity-source'
  GROUP BY source.id
)
INSERT INTO derived_artifacts (
  source_connection_id,document_version_id,store_derived_rights_decision_id,retain_rights_decision_id,
  artifact_type,transformer_profile_sha256,vault_relative_path,mime_type,sha256,size_bytes,
  lifecycle_state,active,available_at,retention_until
)
SELECT decisions.source_id,version.id,decisions.store_id,decisions.retain_id,'plaintext',repeat('5',64),
       format('documents/%s/%s/plaintext/%s.txt',version.document_id,version.id,repeat('5',64)),
       'text/plain; charset=utf-8',version.content_sha256,128,'derived_available',true,now(),
       version.captured_at+interval '365 days'
FROM document_versions AS version
JOIN documents AS document ON document.id=version.document_id
JOIN decisions ON decisions.source_id=document.source_connection_id;

WITH decisions AS (
  SELECT source.id AS source_id,
         max(decision.id) FILTER (WHERE decision.action='store_derived') AS store_id,
         max(decision.id) FILTER (WHERE decision.action='retain') AS retain_id
  FROM source_connections AS source
  JOIN source_rights_decisions AS decision ON decision.source_connection_id=source.id
  WHERE source.name='search-capacity-source'
  GROUP BY source.id
)
INSERT INTO derived_artifacts (
  source_connection_id,document_version_id,store_derived_rights_decision_id,retain_rights_decision_id,
  artifact_type,transformer_profile_sha256,vault_relative_path,mime_type,sha256,size_bytes,
  anchor_normalization_version,anchor_map_profile_version,anchor_plaintext_sha256,anchor_markdown_sha256,
  anchor_map_sha256,lifecycle_state,active,available_at,retention_until
)
SELECT decisions.source_id,version.id,decisions.store_id,decisions.retain_id,'markdown',repeat('6',64),
       format('documents/%s/%s/markdown/%s.md',version.document_id,version.id,repeat('6',64)),
       'text/markdown; charset=utf-8',
       md5('search-capacity-markdown-'||version.id)||md5('search-capacity-markdown-'||version.id),128,
       'canonical-nfc-plaintext-v1','search-capacity-anchor-v1',version.content_sha256,
       md5('search-capacity-markdown-'||version.id)||md5('search-capacity-markdown-'||version.id),
       md5('search-capacity-map-'||version.id)||md5('search-capacity-map-'||version.id),
       'derived_available',true,now(),version.captured_at+interval '365 days'
FROM document_versions AS version
JOIN documents AS document ON document.id=version.document_id
JOIN decisions ON decisions.source_id=document.source_connection_id;

INSERT INTO document_anchor_blocks (
  derived_artifact_id,anchor_map_sha256,block_ordinal,plaintext_utf8_byte_start,plaintext_utf8_byte_end,
  markdown_utf8_byte_start,markdown_utf8_byte_end,markdown_anchor
)
SELECT artifact.id,artifact.anchor_map_sha256,0,0,1,0,1,
       'body-0000-'||left(md5('search-capacity-anchor-'||artifact.id),12)
FROM derived_artifacts AS artifact
JOIN source_connections AS source ON source.id=artifact.source_connection_id
WHERE source.name='search-capacity-source' AND artifact.artifact_type='markdown';

UPDATE document_versions AS version
SET lifecycle_state='derived_available',version=version.version+1,updated_at=now()
FROM documents AS document,source_connections AS source
WHERE version.document_id=document.id AND document.source_connection_id=source.id
  AND source.name='search-capacity-source' AND version.lifecycle_state='derive_pending';

WITH display_decision AS (
  SELECT decision.id
  FROM source_rights_decisions AS decision
  JOIN source_connections AS source ON source.id=decision.source_connection_id
  WHERE source.name='search-capacity-source' AND decision.action='display_private'
)
UPDATE document_versions AS version
SET lifecycle_state='readable',display_private_rights_decision_id=display_decision.id,
    version=version.version+1,updated_at=now()
FROM documents AS document,source_connections AS source,display_decision
WHERE version.document_id=document.id AND document.source_connection_id=source.id
  AND source.name='search-capacity-source' AND version.lifecycle_state='derived_available';

UPDATE documents AS document
SET current_document_version_id=version.id,version=document.version+1,updated_at=now()
FROM document_versions AS version,source_connections AS source
WHERE version.document_id=document.id AND document.source_connection_id=source.id
  AND source.name='search-capacity-source' AND version.lifecycle_state='readable';

INSERT INTO document_version_search_indexes (
  document_version_id,source_connection_id,derived_artifact_id,
  store_derived_rights_decision_id,retain_rights_decision_id,normalization_profile_version,
  normalized_text_sha256,title_search_vector,body_search_vector,title_trigrams,body_trigrams,
  entity_keys,action_keys,location_keys,region_keys,retention_until,indexed_at
)
SELECT version.id,document.source_connection_id,artifact.id,
       artifact.store_derived_rights_decision_id,artifact.retain_rights_decision_id,'search-capacity-v1',
       version.content_sha256,
       to_tsvector('simple','capacitytoken 芯片 release capacity-entity'),
       to_tsvector('simple','capacitytoken multilingual release body capacity-entity'),
       ARRAY['cap','apa','pac'],ARRAY['cap','apa','pac'],ARRAY['capacity-entity'],ARRAY['release'],ARRAY['shanghai'],ARRAY['cn'],
       artifact.retention_until,now()
FROM document_versions AS version
JOIN documents AS document ON document.id=version.document_id
JOIN source_connections AS source ON source.id=document.source_connection_id AND source.name='search-capacity-source'
JOIN derived_artifacts AS artifact ON artifact.document_version_id=version.id AND artifact.artifact_type='plaintext';

WITH generated AS (
  SELECT row_number() OVER (ORDER BY document.id) AS ordinal,document.*,version.captured_at
  FROM documents AS document
  JOIN source_connections AS source ON source.id=document.source_connection_id AND source.name='search-capacity-source'
  JOIN document_versions AS version ON version.id=document.current_document_version_id
)
INSERT INTO contents (
  source_connection_id,external_id,content_type,title,excerpt,canonical_url,language,
  published_at,fetched_at,dedupe_key,content_status
)
SELECT source_connection_id,external_work_id,'article',
       'capacitytoken 芯片 release capacity-entity '||ordinal||CASE WHEN ordinal=1 THEN ' capacityindexstale' ELSE '' END,
       'multilingual PostgreSQL search capacity fixture '||ordinal,
       'https://fixture.invalid/search-capacity/'||ordinal,'zh-CN',captured_at,captured_at,
       md5('search-capacity-dedupe-'||ordinal)||md5('search-capacity-dedupe-'||ordinal),'active'
FROM generated;

INSERT INTO micro_events (
  event_key,status,primary_subject_key,primary_action_key,location_keys,identifier_keys,
  event_started_at,clustering_profile_version
)
SELECT md5('search-capacity-event-'||ordinal)||md5('search-capacity-event-'||ordinal),
       'active','capacitytoken 芯片 subject '||ordinal,'release update '||ordinal,
       ARRAY['shanghai','cn'],ARRAY['capacity-entity'],now()-ordinal*interval '1 second','search-capacity-v1'
FROM generate_series(1,:fixture_rows::integer) AS ordinal;

INSERT INTO events (event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at)
VALUES ('search-capacity-anchor','Search capacity anchor','Synthetic knowledge parent','active',now(),now());

WITH anchor AS (
  SELECT id FROM events WHERE event_key='search-capacity-anchor'
)
INSERT INTO knowledge_documents (
  document_type,event_id,vault_path,revision_no,content_hash,generated_hash,status,last_written_at
)
SELECT 'event',anchor.id,'events/search-capacity-'||ordinal||'.md',1,
       md5('search-capacity-knowledge-'||ordinal)||md5('search-capacity-knowledge-'||ordinal),
       md5('search-capacity-generated-'||ordinal)||md5('search-capacity-generated-'||ordinal),
       'active',now()-ordinal*interval '1 second'
FROM anchor CROSS JOIN generate_series(1,:fixture_rows::integer) AS ordinal;

INSERT INTO knowledge_change_proposals (
  document_id,change_type,base_revision_no,base_hash,proposed_frontmatter,proposed_body,
  diff_summary,reason,status,applied_at,created_at,updated_at
)
SELECT document.id,'create',0,repeat('0',64),
       jsonb_build_object('title','capacitytoken 芯片 release knowledge '||document.id,'entities',jsonb_build_array('capacity-entity')),
       'multilingual PostgreSQL search capacitytoken release body '||document.id,
       'synthetic capacity fixture','synthetic capacity fixture','applied',document.last_written_at,document.last_written_at,document.last_written_at
FROM knowledge_documents AS document
WHERE document.vault_path LIKE 'events/search-capacity-%';

INSERT INTO knowledge_revisions (
  document_id,revision_no,source,proposal_id,previous_hash,new_hash,frontmatter_snapshot,created_at
)
SELECT document.id,1,'proposal',proposal.id,NULL,document.content_hash,proposal.proposed_frontmatter,proposal.applied_at
FROM knowledge_documents AS document
JOIN knowledge_change_proposals AS proposal ON proposal.document_id=document.id AND proposal.status='applied'
WHERE document.vault_path LIKE 'events/search-capacity-%';

ANALYZE contents;
ANALYZE documents;
ANALYZE document_versions;
ANALYZE document_version_search_indexes;
ANALYZE source_rights_decisions;
ANALYZE micro_events;
ANALYZE knowledge_documents;
ANALYZE knowledge_change_proposals;
ANALYZE knowledge_revisions;

COMMIT;
SQL

facts=$(psql "$dsn" -X -A -t -F '|' -v ON_ERROR_STOP=1 <<'SQL'
SELECT
  (SELECT count(*) FROM contents AS content JOIN source_connections AS source ON source.id=content.source_connection_id WHERE source.name='search-capacity-source'),
  (SELECT count(*) FROM micro_events WHERE clustering_profile_version='search-capacity-v1'),
  (SELECT count(*) FROM knowledge_documents WHERE vault_path LIKE 'events/search-capacity-%');
SQL
)
expected="${rows}|${rows}|${rows}"
if test "$facts" != "$expected"; then
  printf '%s\n' "search capacity fixture facts ${facts}, want ${expected}; use a fresh isolated database" >&2
  exit 1
fi
printf '%s\n' "Search capacity fixture verified: content=${rows}, events=${rows}, knowledge=${rows}."
