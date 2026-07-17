package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/pgvector/pgvector-go"
)

var _ application.CandidateReader = (*Repository)(nil)

func (repository *Repository) Lexical(ctx context.Context, contentID int64, limit int) ([]domain.Candidate, error) {
	return repository.queryCandidates(ctx, `
SELECT e.id, e.event_key, 'lexical', LEAST(100, MAX(GREATEST(
  similarity(lower(e.title_zh), lower(c.title)),
  similarity(lower(COALESCE(e.title_en, '')), lower(c.title)),
  similarity(lower(e.summary), lower(c.excerpt)),
  similarity(lower(r.value), lower(concat_ws(' ', c.title, c.excerpt))))) * 100), e.representative_content_id
FROM contents c
JOIN monitor_matches mm ON mm.content_id = c.id AND mm.decision = 'accepted'
JOIN monitor_rules r ON r.config_version_id = mm.monitor_config_version_id
  AND r.enabled AND r.approval_status = 'approved' AND r.operator IN ('contains','equals')
  AND r.rule_type IN ('keyword','phrase','entity')
  AND position(lower(trim(r.value)) IN lower(concat_ws(' ', c.title, c.excerpt))) > 0
JOIN monitor_events me ON me.monitor_id = mm.monitor_id
JOIN events e ON e.id = me.event_id
WHERE c.id = $1 AND c.content_status = 'active'
  AND e.lifecycle_status IN ('detected','active','cooling','closed') AND e.deleted_at IS NULL
GROUP BY e.id, e.event_key, e.representative_content_id
ORDER BY 4 DESC, e.event_key ASC LIMIT $2`, contentID, limit)
}

func (repository *Repository) Temporal(ctx context.Context, contentID int64, limit int) ([]domain.Candidate, error) {
	return repository.queryCandidates(ctx, `
WITH input AS (
  SELECT c.published_at, c.source_connection_id,
         ARRAY(
           SELECT DISTINCT lower(region)
           FROM monitor_matches input_match
           JOIN monitor_config_versions input_config ON input_config.id = input_match.monitor_config_version_id
           CROSS JOIN LATERAL unnest(input_config.regions) AS region
           WHERE input_match.content_id = c.id AND input_match.decision = 'accepted'
           ORDER BY lower(region)
         ) AS regions
  FROM contents c
  WHERE c.id = $1 AND c.content_status = 'active'
)
SELECT e.id, e.event_key, 'temporal', GREATEST(0, 100 - LEAST(100, ABS(EXTRACT(EPOCH FROM (e.last_seen_at - input.published_at))) / 86400.0 * 100 / 30)), e.representative_content_id
FROM input
JOIN events e ON true
WHERE e.lifecycle_status IN ('detected','active','cooling','closed') AND e.deleted_at IS NULL
  AND e.last_seen_at >= input.published_at - interval '30 days'
  AND e.last_seen_at <= input.published_at + interval '30 days'
  AND (
    EXISTS (
      SELECT 1
      FROM event_contents ec
      JOIN contents candidate_content ON candidate_content.id = ec.content_id AND candidate_content.content_status = 'active'
      WHERE ec.event_id = e.id AND candidate_content.source_connection_id = input.source_connection_id
    )
    OR (
      cardinality(input.regions) > 0 AND EXISTS (
        SELECT 1
        FROM monitor_events candidate_monitor_event
        JOIN monitor_config_versions candidate_config ON candidate_config.monitor_id = candidate_monitor_event.monitor_id
        WHERE candidate_monitor_event.event_id = e.id
          AND candidate_config.state IN ('published','superseded')
          AND candidate_config.regions && input.regions
      )
    )
  )
ORDER BY 4 DESC, e.event_key ASC LIMIT $2`, contentID, limit)
}

func (repository *Repository) Fingerprint(ctx context.Context, contentID int64, limit int) ([]domain.Candidate, error) {
	if !repository.available() || contentID <= 0 || limit <= 0 {
		return nil, sharedrepository.ErrUnavailable
	}
	fingerprint, available, err := repository.fingerprintForContent(ctx, repository.runtime.SQL, contentID)
	if err != nil {
		return nil, err
	}
	if !available {
		return []domain.Candidate{}, nil
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT e.id, e.event_key, 'fingerprint', 100, e.representative_content_id
FROM events e
WHERE e.event_fingerprint = $1 AND e.fingerprint_version = $2
  AND e.lifecycle_status IN ('detected','active','cooling','closed') AND e.deleted_at IS NULL
  AND e.first_seen_at >= $3 AND e.first_seen_at < $3 + interval '1 day'
ORDER BY e.event_key ASC LIMIT $4`, fingerprint.Value, fingerprint.Version, fingerprint.TimeBucket, limit)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	return scanCandidates(rows)
}

type candidateQuery interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *Repository) fingerprintForContent(ctx context.Context, query candidateQuery, contentID int64) (domain.EventFingerprint, bool, error) {
	var publishedAt time.Time
	if err := query.QueryRowContext(ctx, `SELECT published_at FROM contents WHERE id = $1 AND content_status = 'active'`, contentID).Scan(&publishedAt); err != nil {
		if err == sql.ErrNoRows {
			return domain.EventFingerprint{}, false, fmt.Errorf("%w: active content", sharedrepository.ErrNotFound)
		}
		return domain.EventFingerprint{}, false, sharedrepository.MapError(err)
	}
	rules, err := query.QueryContext(ctx, `
SELECT r.rule_type, r.value
FROM contents c
JOIN monitor_matches mm ON mm.content_id = c.id AND mm.decision = 'accepted'
JOIN monitor_rules r ON r.config_version_id = mm.monitor_config_version_id
  AND r.enabled AND r.approval_status = 'approved' AND r.operator IN ('contains','equals')
  AND r.rule_type IN ('entity','keyword','phrase')
WHERE c.id = $1
  AND position(lower(trim(r.value)) IN lower(concat_ws(' ', c.title, c.excerpt))) > 0
ORDER BY r.rule_type ASC, lower(r.value) ASC`, contentID)
	if err != nil {
		return domain.EventFingerprint{}, false, sharedrepository.MapError(err)
	}
	defer rules.Close()
	facts := domain.EventFingerprintFacts{PublishedAt: publishedAt}
	for rules.Next() {
		var ruleType, value string
		if err := rules.Scan(&ruleType, &value); err != nil {
			return domain.EventFingerprint{}, false, sharedrepository.MapError(err)
		}
		if ruleType == "entity" {
			facts.EntityTerms = append(facts.EntityTerms, value)
		} else {
			facts.ActionTerms = append(facts.ActionTerms, value)
		}
	}
	if err := rules.Err(); err != nil {
		return domain.EventFingerprint{}, false, sharedrepository.MapError(err)
	}
	regions, err := query.QueryContext(ctx, `
SELECT DISTINCT lower(region)
FROM monitor_matches mm
JOIN monitor_config_versions config ON config.id = mm.monitor_config_version_id
CROSS JOIN LATERAL unnest(config.regions) AS region
WHERE mm.content_id = $1 AND mm.decision = 'accepted'
ORDER BY lower(region) ASC`, contentID)
	if err != nil {
		return domain.EventFingerprint{}, false, sharedrepository.MapError(err)
	}
	defer regions.Close()
	for regions.Next() {
		var region string
		if err := regions.Scan(&region); err != nil {
			return domain.EventFingerprint{}, false, sharedrepository.MapError(err)
		}
		facts.Regions = append(facts.Regions, region)
	}
	if err := regions.Err(); err != nil {
		return domain.EventFingerprint{}, false, sharedrepository.MapError(err)
	}
	fingerprint, available := domain.BuildEventFingerprint(facts)
	return fingerprint, available, nil
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
SELECT e.id, e.event_key, 'vector', (100 - LEAST(100, $1::halfvec <=> ee.embedding) * 100), e.representative_content_id
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
		var representativeContentID sql.NullInt64
		if err := rows.Scan(&candidate.EventID, &candidate.EventKey, &channel, &candidate.Score, &representativeContentID); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		candidate.Channel = domain.CandidateChannel(channel)
		if representativeContentID.Valid {
			candidate.EvidenceContentIDs = []int64{representativeContentID.Int64}
		}
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return result, nil
}
