package scorerepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SaveScore(ctx context.Context, score servicehotspot.HotspotScore) (servicehotspot.HotspotScore, error) {
	const query = `
INSERT INTO hotspot_scores (
	id, cluster_id, total_score, source_count_score, freshness_score, relevance_score, propagation_score, quality_score, explanation, score_version, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (cluster_id, score_version) DO UPDATE SET
	total_score = EXCLUDED.total_score,
	source_count_score = EXCLUDED.source_count_score,
	freshness_score = EXCLUDED.freshness_score,
	relevance_score = EXCLUDED.relevance_score,
	propagation_score = EXCLUDED.propagation_score,
	quality_score = EXCLUDED.quality_score,
	explanation = EXCLUDED.explanation,
	updated_at = EXCLUDED.updated_at
RETURNING id, cluster_id, total_score, source_count_score, freshness_score, relevance_score, propagation_score, quality_score, explanation, score_version, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, query,
		score.ID, score.ClusterID, score.TotalScore, score.SourceCountScore, score.FreshnessScore, score.RelevanceScore, score.PropagationScore, score.QualityScore, score.Explanation, score.ScoreVersion, score.CreatedAt, score.UpdatedAt,
	).Scan(
		&score.ID, &score.ClusterID, &score.TotalScore, &score.SourceCountScore, &score.FreshnessScore, &score.RelevanceScore, &score.PropagationScore, &score.QualityScore, &score.Explanation, &score.ScoreVersion, &score.CreatedAt, &score.UpdatedAt,
	)
	if err != nil {
		return servicehotspot.HotspotScore{}, err
	}

	// Hydrate ChannelIDs and SourceRefs from explanation if possible, or leave to caller to join
	hydrateFromExplanation(&score)

	return score, nil
}

func (r *Repository) ListScores(ctx context.Context) ([]servicehotspot.HotspotScore, error) {
	const query = `
SELECT id, cluster_id, total_score, source_count_score, freshness_score, relevance_score, propagation_score, quality_score, explanation, score_version, created_at, updated_at
FROM hotspot_scores
ORDER BY total_score DESC, updated_at DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scores []servicehotspot.HotspotScore
	for rows.Next() {
		var s servicehotspot.HotspotScore
		if err := rows.Scan(
			&s.ID, &s.ClusterID, &s.TotalScore, &s.SourceCountScore, &s.FreshnessScore, &s.RelevanceScore, &s.PropagationScore, &s.QualityScore, &s.Explanation, &s.ScoreVersion, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		hydrateFromExplanation(&s)
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

func (r *Repository) ListScoresByWindow(ctx context.Context, start, end time.Time) ([]servicehotspot.HotspotScore, error) {
	const query = `
SELECT id, cluster_id, total_score, source_count_score, freshness_score, relevance_score, propagation_score, quality_score, explanation, score_version, created_at, updated_at
FROM hotspot_scores
WHERE created_at >= $1 AND created_at < $2
ORDER BY total_score DESC, updated_at DESC`
	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scores []servicehotspot.HotspotScore
	for rows.Next() {
		var s servicehotspot.HotspotScore
		if err := rows.Scan(
			&s.ID, &s.ClusterID, &s.TotalScore, &s.SourceCountScore, &s.FreshnessScore, &s.RelevanceScore, &s.PropagationScore, &s.QualityScore, &s.Explanation, &s.ScoreVersion, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		hydrateFromExplanation(&s)
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

func (r *Repository) FindScoreByClusterID(ctx context.Context, clusterID string) (servicehotspot.HotspotScore, error) {
	const query = `
SELECT id, cluster_id, total_score, source_count_score, freshness_score, relevance_score, propagation_score, quality_score, explanation, score_version, created_at, updated_at
FROM hotspot_scores
WHERE cluster_id = $1
ORDER BY updated_at DESC
LIMIT 1`
	var s servicehotspot.HotspotScore
	err := r.db.QueryRowContext(ctx, query, clusterID).Scan(
		&s.ID, &s.ClusterID, &s.TotalScore, &s.SourceCountScore, &s.FreshnessScore, &s.RelevanceScore, &s.PropagationScore, &s.QualityScore, &s.Explanation, &s.ScoreVersion, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return servicehotspot.HotspotScore{}, servicehotspot.ErrScoreNotFound
	}
	if err != nil {
		return servicehotspot.HotspotScore{}, err
	}
	hydrateFromExplanation(&s)
	return s, nil
}

func hydrateFromExplanation(s *servicehotspot.HotspotScore) {
	if s.Explanation == "" {
		return
	}
	var exp servicehotspot.ScoreExplanation
	if err := json.Unmarshal([]byte(s.Explanation), &exp); err == nil {
		s.SourceRefs = exp.SourceRefs
		// Note: ChannelIDs are not currently in ScoreExplanation,
		// but they could be added if needed for persistence without joining.
	}
}
