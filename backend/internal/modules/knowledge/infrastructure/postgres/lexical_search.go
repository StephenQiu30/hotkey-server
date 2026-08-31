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

// Search reads the current applied proposal for each active knowledge
// document. Source and monitor filters require projections owned by other
// modules, so this owner-local reader fails closed for those filters.
func (repository *Repository) Search(ctx context.Context, query searchdomain.Query) ([]searchdomain.Candidate, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	if !query.Includes(searchdomain.ResourceKnowledge) || query.SourceConnectionID != nil || query.MonitorID != nil {
		return []searchdomain.Candidate{}, nil
	}
	hasAfter, afterScore, afterOccurredAt, afterType, afterID := knowledgeSearchPageArguments(query)
	rows, err := repository.runtime.SQL.QueryContext(ctx, knowledgeLexicalSearchSQL,
		query.Keyword, query.Entity, query.Status, knowledgeSearchNullableTime(query.From),
		knowledgeSearchNullableTime(query.To), query.Sort, knowledgeSearchSnapshot(query.SnapshotAt),
		hasAfter, afterScore, afterOccurredAt, afterType, afterID, query.CandidateLimit,
	)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]searchdomain.Candidate, 0, query.Limit)
	for rows.Next() {
		candidate := searchdomain.Candidate{Type: searchdomain.ResourceKnowledge}
		if err := rows.Scan(&candidate.ID, &candidate.Title, &candidate.Snippet, &candidate.Status, &candidate.OccurredAt, &candidate.Score); err != nil {
			return nil, databaserepository.MapError(err)
		}
		candidate.OccurredAt = candidate.OccurredAt.UTC()
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("%w: invalid knowledge search projection", sharedrepository.ErrConflict)
		}
		items = append(items, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return items, nil
}

func (repository *Repository) CanDisplay(ctx context.Context, query searchdomain.Query, candidate searchdomain.Candidate) (bool, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return false, sharedrepository.ErrUnavailable
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil || candidate.Type != searchdomain.ResourceKnowledge || candidate.ID <= 0 {
		return false, fmt.Errorf("%w: invalid knowledge visibility query", sharedrepository.ErrInvalidInput)
	}
	if query.SourceConnectionID != nil || query.MonitorID != nil {
		return false, nil
	}
	var visible bool
	err := repository.runtime.SQL.QueryRowContext(ctx, knowledgeSearchVisibilitySQL,
		candidate.ID, query.Status, knowledgeSearchNullableTime(query.From), knowledgeSearchNullableTime(query.To), query.Entity,
	).Scan(&visible)
	if err != nil {
		return false, databaserepository.MapError(err)
	}
	return visible, nil
}

// ExplainSearch returns PostgreSQL's non-ANALYZE JSON plan for the exact
// production query. Acceptance tooling must sanitize it before persistence.
func (repository *Repository) ExplainSearch(ctx context.Context, query searchdomain.Query) ([]byte, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil || !query.Includes(searchdomain.ResourceKnowledge) || query.SourceConnectionID != nil || query.MonitorID != nil {
		return nil, fmt.Errorf("%w: invalid knowledge search plan query", sharedrepository.ErrInvalidInput)
	}
	var plan []byte
	hasAfter, afterScore, afterOccurredAt, afterType, afterID := knowledgeSearchPageArguments(query)
	err := repository.runtime.SQL.QueryRowContext(ctx, "EXPLAIN (FORMAT JSON,COSTS FALSE) "+knowledgeLexicalSearchSQL,
		query.Keyword, query.Entity, query.Status, knowledgeSearchNullableTime(query.From),
		knowledgeSearchNullableTime(query.To), query.Sort, knowledgeSearchSnapshot(query.SnapshotAt),
		hasAfter, afterScore, afterOccurredAt, afterType, afterID, query.CandidateLimit,
	).Scan(&plan)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	return append([]byte(nil), plan...), nil
}

func knowledgeSearchNullableTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func knowledgeSearchSnapshot(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func knowledgeSearchPageArguments(query searchdomain.Query) (bool, float64, time.Time, string, int64) {
	if query.After == nil {
		return false, 0, time.Time{}, "", 0
	}
	return true, query.After.Score, query.After.OccurredAt.UTC(), string(query.After.Type), query.After.ID
}

const knowledgeLexicalSearchSQL = `
WITH input AS MATERIALIZED (
  SELECT websearch_to_tsquery('simple',$1::text) AS terms,
         lower($1::text) AS keyword,
         lower($2::text) AS entity
), projections AS (
  SELECT document.id,
         COALESCE(NULLIF(proposal.proposed_frontmatter->>'title',''),document.document_type || ' #' || document.id::text) AS title,
         left(proposal.proposed_body,8192) AS snippet,
         document.status,
         revision.created_at AS occurred_at,
         COALESCE(proposal.proposed_frontmatter->>'title','') || ' ' || proposal.proposed_body AS haystack,
         proposal.proposed_frontmatter
  FROM knowledge_documents AS document
  JOIN knowledge_revisions AS revision
    ON revision.document_id=document.id AND revision.revision_no=document.revision_no
  JOIN knowledge_change_proposals AS proposal
    ON proposal.id=revision.proposal_id AND proposal.status='applied'
  WHERE document.status='active'
    AND ($7::timestamptz IS NULL OR document.created_at<=$7 AND revision.created_at<=$7)
    AND ($3::text='' OR document.status=$3)
    AND ($4::timestamptz IS NULL OR revision.created_at>=$4)
    AND ($5::timestamptz IS NULL OR revision.created_at<=$5)
    AND ($2::text='' OR EXISTS (
      SELECT 1 FROM jsonb_array_elements_text(
        CASE WHEN jsonb_typeof(proposal.proposed_frontmatter->'entities')='array'
          THEN proposal.proposed_frontmatter->'entities' ELSE '[]'::jsonb END
      ) AS entity_key WHERE lower(entity_key)=lower($2::text)
    ))
), candidates AS (
  SELECT projection.*,
         LEAST(100,GREATEST(
           ts_rank_cd(to_tsvector('simple',projection.haystack),input.terms,32)*4,
           similarity(lower(projection.haystack),input.keyword),
           CASE WHEN strpos(lower(projection.haystack),input.keyword)>0 THEN 0.8 ELSE 0 END
         ))::double precision AS score
  FROM projections AS projection CROSS JOIN input
  WHERE to_tsvector('simple',projection.haystack) @@ input.terms
     OR lower(projection.haystack) % input.keyword
     OR strpos(lower(projection.haystack),input.keyword)>0
)
SELECT id,title,snippet,status,occurred_at,score
FROM candidates
WHERE $8::boolean=false OR (
  $6::text='relevance' AND (
    score<$9 OR score=$9 AND (
      occurred_at<$10 OR occurred_at=$10 AND (
        'knowledge'::text>$11 OR 'knowledge'::text=$11 AND id<$12
      )
    )
  )
  OR $6::text='latest' AND (
    occurred_at<$10 OR occurred_at=$10 AND (
      'knowledge'::text>$11 OR 'knowledge'::text=$11 AND (
        score<$9 OR score=$9 AND id<$12
      )
    )
  )
)
ORDER BY
  CASE WHEN $6::text='relevance' THEN score END DESC,
  CASE WHEN $6::text='latest' THEN occurred_at END DESC,
  CASE WHEN $6::text='relevance' THEN occurred_at END DESC,
  CASE WHEN $6::text='latest' THEN score END DESC,
  id DESC
LIMIT $13`

const knowledgeSearchVisibilitySQL = `
SELECT EXISTS (
  SELECT 1 FROM knowledge_documents AS document
  JOIN knowledge_revisions AS revision
    ON revision.document_id=document.id AND revision.revision_no=document.revision_no
  JOIN knowledge_change_proposals AS proposal
    ON proposal.id=revision.proposal_id AND proposal.status='applied'
  WHERE document.id=$1 AND document.status='active'
    AND ($2::text='' OR document.status=$2)
    AND ($3::timestamptz IS NULL OR revision.created_at>=$3)
    AND ($4::timestamptz IS NULL OR revision.created_at<=$4)
    AND ($5::text='' OR EXISTS (
      SELECT 1 FROM jsonb_array_elements_text(
        CASE WHEN jsonb_typeof(proposal.proposed_frontmatter->'entities')='array'
          THEN proposal.proposed_frontmatter->'entities' ELSE '[]'::jsonb END
      ) AS entity_key WHERE lower(entity_key)=lower($5::text)
    ))
)`
