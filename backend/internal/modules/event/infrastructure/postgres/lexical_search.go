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

// Search exposes the bounded lexical projection owned by the event module.
// Source filtering deliberately fails closed here: resolving a source requires
// an ingestion-owned projection and is composed by the search application.
func (repository *MicroEventQueryPostgresRepository) Search(ctx context.Context, query searchdomain.Query) ([]searchdomain.Candidate, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	if !query.Includes(searchdomain.ResourceEvent) || query.SourceConnectionID != nil {
		return []searchdomain.Candidate{}, nil
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, eventLexicalSearchSQL,
		query.Keyword, query.Entity, eventSearchNullableID(query.MonitorID), query.Status,
		eventSearchNullableTime(query.From), eventSearchNullableTime(query.To), query.Limit,
	)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	items := make([]searchdomain.Candidate, 0, query.Limit)
	for rows.Next() {
		candidate := searchdomain.Candidate{Type: searchdomain.ResourceEvent}
		if err := rows.Scan(&candidate.ID, &candidate.Title, &candidate.Snippet, &candidate.Status, &candidate.OccurredAt, &candidate.Score); err != nil {
			return nil, databaserepository.MapError(err)
		}
		candidate.OccurredAt = candidate.OccurredAt.UTC()
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("%w: invalid event search projection", sharedrepository.ErrConflict)
		}
		items = append(items, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return items, nil
}

func (repository *MicroEventQueryPostgresRepository) CanDisplay(ctx context.Context, query searchdomain.Query, candidate searchdomain.Candidate) (bool, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return false, sharedrepository.ErrUnavailable
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil || candidate.Type != searchdomain.ResourceEvent || candidate.ID <= 0 {
		return false, fmt.Errorf("%w: invalid event visibility query", sharedrepository.ErrInvalidInput)
	}
	if query.SourceConnectionID != nil {
		return false, nil
	}
	var visible bool
	err := repository.runtime.SQL.QueryRowContext(ctx, eventSearchVisibilitySQL,
		candidate.ID, eventSearchNullableID(query.MonitorID), query.Status,
		eventSearchNullableTime(query.From), eventSearchNullableTime(query.To), query.Entity,
	).Scan(&visible)
	if err != nil {
		return false, databaserepository.MapError(err)
	}
	return visible, nil
}

// ExplainSearch returns PostgreSQL's non-ANALYZE JSON plan for the exact
// production query. Acceptance tooling must sanitize it before persistence.
func (repository *MicroEventQueryPostgresRepository) ExplainSearch(ctx context.Context, query searchdomain.Query) ([]byte, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	query = query.Normalized()
	if err := query.Validate(); err != nil || !query.Includes(searchdomain.ResourceEvent) || query.SourceConnectionID != nil {
		return nil, fmt.Errorf("%w: invalid event search plan query", sharedrepository.ErrInvalidInput)
	}
	var plan []byte
	err := repository.runtime.SQL.QueryRowContext(ctx, "EXPLAIN (FORMAT JSON,COSTS FALSE) "+eventLexicalSearchSQL,
		query.Keyword, query.Entity, eventSearchNullableID(query.MonitorID), query.Status,
		eventSearchNullableTime(query.From), eventSearchNullableTime(query.To), query.Limit,
	).Scan(&plan)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	return append([]byte(nil), plan...), nil
}

func eventSearchNullableID(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func eventSearchNullableTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}

const eventLexicalSearchSQL = `
WITH input AS MATERIALIZED (
  SELECT websearch_to_tsquery('simple',$1::text) AS terms,
         lower($1::text) AS keyword,
         lower($2::text) AS entity
), projections AS (
  SELECT event.id,event.status,event.event_started_at,
         event.primary_subject_key || ' · ' || event.primary_action_key AS title,
         COALESCE(NULLIF(summary.text,''),
           concat_ws(' ',array_to_string(event.location_keys,' '),array_to_string(event.identifier_keys,' '))) AS snippet,
         event.primary_subject_key || ' ' || event.primary_action_key || ' ' ||
           array_to_string(event.location_keys,' ') || ' ' || array_to_string(event.identifier_keys,' ') || ' ' ||
           COALESCE(summary.text,'') AS haystack
  FROM micro_events AS event
  LEFT JOIN LATERAL (
    SELECT string_agg(sentence.sentence,' ' ORDER BY sentence.ordinal) AS text
    FROM micro_event_summaries AS summary
    JOIN micro_event_summary_sentences AS sentence ON sentence.micro_event_summary_id=summary.id
    WHERE summary.id=(
      SELECT current_summary.id FROM micro_event_summaries AS current_summary
      WHERE current_summary.micro_event_id=event.id AND current_summary.micro_event_version=event.version
      ORDER BY current_summary.created_at DESC,current_summary.id DESC LIMIT 1
    )
  ) AS summary ON true
  WHERE ($3::bigint IS NULL OR EXISTS (
    SELECT 1 FROM micro_event_membership_decisions AS decision
    WHERE decision.resulting_micro_event_id=event.id AND decision.monitor_id=$3
  ))
    AND ($4::text='' OR event.status=$4)
    AND ($5::timestamptz IS NULL OR event.event_started_at>=$5)
    AND ($6::timestamptz IS NULL OR event.event_started_at<=$6)
    AND ($2::text='' OR lower(event.primary_subject_key)=lower($2::text)
      OR EXISTS (SELECT 1 FROM unnest(event.location_keys || event.identifier_keys) AS entity_key WHERE lower(entity_key)=lower($2::text)))
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
SELECT id,title,snippet,status,event_started_at,score
FROM candidates
ORDER BY score DESC,event_started_at DESC,id DESC
LIMIT $7`

const eventSearchVisibilitySQL = `
SELECT EXISTS (
  SELECT 1 FROM micro_events AS event
  WHERE event.id=$1
    AND ($2::bigint IS NULL OR EXISTS (
      SELECT 1 FROM micro_event_membership_decisions AS decision
      WHERE decision.resulting_micro_event_id=event.id AND decision.monitor_id=$2
    ))
    AND ($3::text='' OR event.status=$3)
    AND ($4::timestamptz IS NULL OR event.event_started_at>=$4)
    AND ($5::timestamptz IS NULL OR event.event_started_at<=$5)
    AND ($6::text='' OR lower(event.primary_subject_key)=lower($6::text)
      OR EXISTS (SELECT 1 FROM unnest(event.location_keys || event.identifier_keys) AS entity_key WHERE lower(entity_key)=lower($6::text)))
)`
