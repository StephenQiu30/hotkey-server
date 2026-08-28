package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// Search reads only ingestion-owned Content/Document lexical projections.
// The canonical body never leaves its tsvector/trigram projection; display
// fields remain the bounded Content title and excerpt.
func (repository *ContentRepository) Search(ctx context.Context, query searchdomain.Query) ([]searchdomain.Candidate, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	if !query.Includes(searchdomain.ResourceContent) {
		return []searchdomain.Candidate{}, nil
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, contentLexicalSearchSQL,
		query.Keyword, query.Entity, nullableSearchID(query.SourceConnectionID), nullableSearchID(query.MonitorID),
		query.Status, nullableSearchTime(query.From), nullableSearchTime(query.To), query.Limit,
	)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	items := make([]searchdomain.Candidate, 0, query.Limit)
	for rows.Next() {
		var candidate searchdomain.Candidate
		candidate.Type = searchdomain.ResourceContent
		if err := rows.Scan(&candidate.ID, &candidate.SourceConnectionID, &candidate.Title, &candidate.Snippet, &candidate.Status, &candidate.OccurredAt, &candidate.Score); err != nil {
			return nil, databaserepository.MapError(err)
		}
		candidate.OccurredAt = candidate.OccurredAt.UTC()
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("%w: invalid content search projection", sharedrepository.ErrConflict)
		}
		items = append(items, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return items, nil
}

// CanDisplay rechecks current lifecycle, rights, retention and object filters
// immediately before the search application assembles a response DTO.
func (repository *ContentRepository) CanDisplay(ctx context.Context, query searchdomain.Query, candidate searchdomain.Candidate) (bool, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return false, sharedrepository.ErrUnavailable
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil || candidate.Type != searchdomain.ResourceContent || candidate.ID <= 0 {
		return false, fmt.Errorf("%w: invalid content visibility query", sharedrepository.ErrInvalidInput)
	}
	var visible bool
	err := repository.runtime.SQL.QueryRowContext(ctx, contentSearchVisibilitySQL,
		candidate.ID, nullableSearchID(query.SourceConnectionID), nullableSearchID(query.MonitorID), query.Status,
		nullableSearchTime(query.From), nullableSearchTime(query.To), query.Entity,
	).Scan(&visible)
	if err != nil {
		return false, databaserepository.MapError(err)
	}
	return visible, nil
}

// ExplainSearch returns PostgreSQL's non-ANALYZE JSON plan for the exact
// production query. Acceptance tooling must sanitize it before persistence.
func (repository *ContentRepository) ExplainSearch(ctx context.Context, query searchdomain.Query) ([]byte, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil || !query.Includes(searchdomain.ResourceContent) {
		return nil, fmt.Errorf("%w: invalid content search plan query", sharedrepository.ErrInvalidInput)
	}
	var plan []byte
	err := repository.runtime.SQL.QueryRowContext(ctx, "EXPLAIN (FORMAT JSON,COSTS FALSE) "+contentLexicalSearchSQL,
		query.Keyword, query.Entity, nullableSearchID(query.SourceConnectionID), nullableSearchID(query.MonitorID),
		query.Status, nullableSearchTime(query.From), nullableSearchTime(query.To), query.Limit,
	).Scan(&plan)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	return append([]byte(nil), plan...), nil
}

func nullableSearchID(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func nullableSearchTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}

const contentLexicalSearchSQL = `
WITH input AS MATERIALIZED (
  SELECT websearch_to_tsquery('simple',$1::text) AS terms,
         lower($1::text) AS keyword,
         lower($2::text) AS entity
), candidates AS (
  SELECT content.id,content.source_connection_id,content.title,content.excerpt,'active'::text AS status,
         COALESCE(content.published_at,content.fetched_at) AS occurred_at,
         GREATEST(
           ts_rank_cd(search.title_search_vector,input.terms,32)*4,
           ts_rank_cd(search.body_search_vector,input.terms,32)*2,
           similarity(lower(content.title || ' ' || content.excerpt),input.keyword),
           CASE WHEN strpos(lower(content.title || ' ' || content.excerpt),input.keyword)>0 THEN 0.8 ELSE 0 END
         )::double precision AS score
  FROM contents AS content
  JOIN documents AS document
    ON document.source_connection_id=content.source_connection_id
   AND document.external_work_id=content.external_id
   AND document.document_state='active'
  JOIN document_versions AS version
    ON version.id=document.current_document_version_id
   AND version.lifecycle_state='readable'
  JOIN document_version_search_indexes AS search
    ON search.document_version_id=version.id
   AND search.source_connection_id=document.source_connection_id
   AND search.normalized_text_sha256=version.content_sha256
   AND search.lifecycle_state='active'
   AND search.retention_until>CURRENT_TIMESTAMP
  CROSS JOIN input
  WHERE content.content_status='active' AND content.deleted_at IS NULL
    AND current_rights_action_allowed(
      version.display_private_rights_decision_id,document.source_connection_id,
      'document_version',version.id::text,version.content_sha256,'display_private',CURRENT_TIMESTAMP
    )
    AND current_rights_action_allowed(
      search.store_derived_rights_decision_id,search.source_connection_id,
      'document_version',search.document_version_id::text,search.normalized_text_sha256,'store_derived',CURRENT_TIMESTAMP
    )
    AND current_rights_action_allowed(
      search.retain_rights_decision_id,search.source_connection_id,
      'document_version',search.document_version_id::text,search.normalized_text_sha256,'retain',CURRENT_TIMESTAMP
    )
    AND (
      search.title_search_vector @@ input.terms OR search.body_search_vector @@ input.terms
      OR lower(content.title || ' ' || content.excerpt) % input.keyword
      OR strpos(lower(content.title || ' ' || content.excerpt),input.keyword)>0
    )
    AND ($2::text='' OR EXISTS (
      SELECT 1 FROM unnest(search.entity_keys) AS entity_key WHERE lower(entity_key)=input.entity
    ))
    AND ($3::bigint IS NULL OR content.source_connection_id=$3)
    AND ($4::bigint IS NULL OR EXISTS (
      SELECT 1 FROM monitor_matches AS match
      WHERE match.monitor_id=$4 AND match.content_id=content.id AND match.decision IN ('accepted','review')
    ))
    AND ($5::text='' OR $5='active')
    AND ($6::timestamptz IS NULL OR COALESCE(content.published_at,content.fetched_at)>=$6)
    AND ($7::timestamptz IS NULL OR COALESCE(content.published_at,content.fetched_at)<=$7)
), deduplicated AS (
  SELECT id,source_connection_id,title,excerpt,status,occurred_at,max(score) AS score
  FROM candidates GROUP BY id,source_connection_id,title,excerpt,status,occurred_at
)
SELECT id,source_connection_id,title,excerpt,status,occurred_at,score
FROM deduplicated
ORDER BY score DESC,occurred_at DESC,id DESC
LIMIT $8`

const contentSearchVisibilitySQL = `
SELECT EXISTS (
  SELECT 1
  FROM contents AS content
  JOIN documents AS document
    ON document.source_connection_id=content.source_connection_id
   AND document.external_work_id=content.external_id AND document.document_state='active'
  JOIN document_versions AS version
    ON version.id=document.current_document_version_id AND version.lifecycle_state='readable'
  JOIN document_version_search_indexes AS search
    ON search.document_version_id=version.id AND search.source_connection_id=document.source_connection_id
   AND search.normalized_text_sha256=version.content_sha256 AND search.lifecycle_state='active'
   AND search.retention_until>CURRENT_TIMESTAMP
  WHERE content.id=$1 AND content.content_status='active' AND content.deleted_at IS NULL
    AND current_rights_action_allowed(version.display_private_rights_decision_id,document.source_connection_id,
      'document_version',version.id::text,version.content_sha256,'display_private',CURRENT_TIMESTAMP)
    AND current_rights_action_allowed(search.store_derived_rights_decision_id,search.source_connection_id,
      'document_version',search.document_version_id::text,search.normalized_text_sha256,'store_derived',CURRENT_TIMESTAMP)
    AND current_rights_action_allowed(search.retain_rights_decision_id,search.source_connection_id,
      'document_version',search.document_version_id::text,search.normalized_text_sha256,'retain',CURRENT_TIMESTAMP)
    AND ($2::bigint IS NULL OR content.source_connection_id=$2)
    AND ($3::bigint IS NULL OR EXISTS (
      SELECT 1 FROM monitor_matches AS match
      WHERE match.monitor_id=$3 AND match.content_id=content.id AND match.decision IN ('accepted','review')
    ))
    AND ($4::text='' OR $4='active')
    AND ($5::timestamptz IS NULL OR COALESCE(content.published_at,content.fetched_at)>=$5)
    AND ($6::timestamptz IS NULL OR COALESCE(content.published_at,content.fetched_at)<=$6)
    AND ($7::text='' OR EXISTS (SELECT 1 FROM unnest(search.entity_keys) AS entity_key WHERE lower(entity_key)=lower($7::text)))
)`
