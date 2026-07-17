package postgres

import (
	"context"
	"database/sql"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/pgvector/pgvector-go"
)

var _ application.CandidateReader = (*Repository)(nil)

func (repository *Repository) Lexical(ctx context.Context, contentID int64, limit int) ([]domain.Candidate, error) {
	return repository.queryCandidates(ctx, `
SELECT e.id, e.event_key, 'lexical', LEAST(100, GREATEST(
  similarity(lower(e.title_zh), lower(c.title)),
  similarity(lower(COALESCE(e.title_en, '')), lower(c.title)),
  similarity(lower(e.summary), lower(c.excerpt))) * 100)
FROM contents c
JOIN monitor_matches mm ON mm.content_id = c.id AND mm.decision = 'accepted'
JOIN monitor_events me ON me.monitor_id = mm.monitor_id
JOIN events e ON e.id = me.event_id
WHERE c.id = $1 AND c.content_status = 'active'
  AND e.lifecycle_status IN ('detected','active','cooling','closed') AND e.deleted_at IS NULL
ORDER BY 4 DESC, e.event_key ASC LIMIT $2`, contentID, limit)
}

func (repository *Repository) Temporal(ctx context.Context, contentID int64, limit int) ([]domain.Candidate, error) {
	return repository.queryCandidates(ctx, `
SELECT e.id, e.event_key, 'temporal', GREATEST(0, 100 - LEAST(100, ABS(EXTRACT(EPOCH FROM (e.last_seen_at - c.published_at))) / 86400.0 * 100 / 30))
FROM contents c
JOIN monitor_matches mm ON mm.content_id = c.id AND mm.decision = 'accepted'
JOIN monitor_events me ON me.monitor_id = mm.monitor_id
JOIN events e ON e.id = me.event_id
WHERE c.id = $1 AND c.content_status = 'active'
  AND e.lifecycle_status IN ('detected','active','cooling','closed') AND e.deleted_at IS NULL
  AND e.last_seen_at >= c.published_at - interval '30 days'
  AND e.last_seen_at <= c.published_at + interval '30 days'
ORDER BY 4 DESC, e.event_key ASC LIMIT $2`, contentID, limit)
}

func (repository *Repository) Fingerprint(ctx context.Context, contentID int64, limit int) ([]domain.Candidate, error) {
	return repository.queryCandidates(ctx, `
SELECT e.id, e.event_key, 'fingerprint', CASE WHEN e.event_fingerprint IS NOT NULL AND left(e.event_fingerprint, 8) = left(c.dedupe_key, 8) THEN 100 ELSE 0 END
FROM contents c
JOIN monitor_matches mm ON mm.content_id = c.id AND mm.decision = 'accepted'
JOIN monitor_events me ON me.monitor_id = mm.monitor_id
JOIN events e ON e.id = me.event_id
WHERE c.id = $1 AND c.content_status = 'active'
  AND e.lifecycle_status IN ('detected','active','cooling','closed') AND e.deleted_at IS NULL
  AND e.event_fingerprint IS NOT NULL
ORDER BY 4 DESC, e.event_key ASC LIMIT $2`, contentID, limit)
}

func (repository *Repository) Vector(ctx context.Context, contentID int64, limit int) ([]domain.Candidate, error) {
	if !repository.available() || contentID <= 0 {
		return nil, sharedrepository.ErrUnavailable
	}
	var vector pgvector.HalfVector
	var profileID, profileVersion int64
	var modelVersion string
	if err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT ce.embedding, ce.model_profile_id, ce.model_profile_version, ce.model_version
FROM content_embeddings ce
JOIN ai_model_profiles p ON p.id = ce.model_profile_id
WHERE ce.content_id = $1 AND ce.active
  AND p.enabled AND p.deleted_at IS NULL AND p.version = ce.model_profile_version`, contentID).Scan(&vector, &profileID, &profileVersion, &modelVersion); err == sql.ErrNoRows {
		return nil, sharedrepository.ErrUnavailable
	} else if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	var available bool
	if err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM event_embeddings ee
  JOIN ai_model_profiles p ON p.id = ee.model_profile_id
  JOIN events e ON e.id = ee.event_id
  WHERE ee.active AND ee.model_profile_id = $1 AND ee.model_profile_version = $2 AND ee.model_version = $3
    AND p.enabled AND p.deleted_at IS NULL AND p.version = ee.model_profile_version
    AND e.lifecycle_status IN ('detected','active','cooling','closed') AND e.deleted_at IS NULL
)`, profileID, profileVersion, modelVersion).Scan(&available); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	if !available {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT e.id, e.event_key, 'vector', (100 - LEAST(100, $1::halfvec <=> ee.embedding) * 100)
FROM event_embeddings ee
JOIN ai_model_profiles p ON p.id = ee.model_profile_id
JOIN events e ON e.id = ee.event_id
WHERE ee.active AND ee.model_profile_id = $2 AND ee.model_profile_version = $3 AND ee.model_version = $4
  AND p.enabled AND p.deleted_at IS NULL AND p.version = ee.model_profile_version
  AND e.lifecycle_status IN ('detected','active','cooling','closed') AND e.deleted_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM monitor_events me
    JOIN monitor_matches mm ON mm.monitor_id = me.monitor_id AND mm.content_id = $5 AND mm.decision = 'accepted'
    WHERE me.event_id = ee.event_id
  )
ORDER BY ee.embedding <=> $1::halfvec LIMIT $6`, vector, profileID, profileVersion, modelVersion, contentID, limit)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func (repository *Repository) queryCandidates(ctx context.Context, query string, contentID int64, limit int) ([]domain.Candidate, error) {
	if !repository.available() || contentID <= 0 || limit <= 0 {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, query, contentID, limit)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func scanCandidates(rows *sql.Rows) ([]domain.Candidate, error) {
	result := make([]domain.Candidate, 0)
	for rows.Next() {
		var candidate domain.Candidate
		var channel string
		if err := rows.Scan(&candidate.EventID, &candidate.EventKey, &channel, &candidate.Score); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		candidate.Channel = domain.CandidateChannel(channel)
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return result, nil
}
