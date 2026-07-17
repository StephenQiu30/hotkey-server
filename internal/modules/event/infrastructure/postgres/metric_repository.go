package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type metricQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (repository *Repository) SaveHeatSnapshot(ctx context.Context, result domain.HeatResult) error {
	if result.WindowHours == 0 {
		result.WindowHours = 24
	}
	if result.HeatVersion == "" {
		result.HeatVersion = "v1"
	}
	if result.EvidenceSetHash == "" {
		result.EvidenceSetHash = strings.Repeat("0", 64)
	}
	if result.CapabilityProfileSetHash == "" {
		result.CapabilityProfileSetHash = strings.Repeat("0", 64)
	}
	if result.TrendStatus == "" {
		result.TrendStatus = domain.TrendStable
	}
	return repository.SaveRecomputedHeatSnapshots(ctx, []domain.HeatResult{result})
}

func (repository *Repository) LatestHeatSnapshot(ctx context.Context, eventID int64) (domain.HeatResult, error) {
	if !repository.available() || eventID <= 0 {
		return domain.HeatResult{}, sharedrepository.ErrUnavailable
	}
	var result domain.HeatResult
	var reasons []byte
	err := repository.metricQuery(ctx).QueryRowContext(ctx, `
SELECT snapshot.event_id, snapshot.captured_at, snapshot.window_hours, snapshot.heat_score, snapshot.trend_score, snapshot.trend_status, snapshot.source_count, snapshot.content_count,
       snapshot.heat_version, snapshot.evidence_set_hash, snapshot.capability_profile_set_hash, array_to_json(event.heat_reason_codes)
FROM event_metric_snapshots snapshot
JOIN events event ON event.id = snapshot.event_id
WHERE snapshot.event_id = $1
ORDER BY snapshot.captured_at DESC, snapshot.window_hours DESC, snapshot.id DESC
LIMIT 1`, eventID).Scan(&result.EventID, &result.WindowEnd, &result.WindowHours, &result.HeatScore, &result.TrendScore, &result.TrendStatus, &result.SourceCount, &result.ContentCount, &result.HeatVersion, &result.EvidenceSetHash, &result.CapabilityProfileSetHash, &reasons)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.HeatResult{}, sharedrepository.ErrNotFound
		}
		return domain.HeatResult{}, sharedrepository.MapError(err)
	}
	if err := json.Unmarshal(reasons, &result.ReasonCodes); err != nil {
		return domain.HeatResult{}, sharedrepository.MapError(err)
	}
	if result.ReasonCodes == nil {
		result.ReasonCodes = []string{}
	}
	return result, nil
}

func (repository *Repository) LoadMetricEvidence(ctx context.Context, eventID int64, windowEnd time.Time, windowHours int) (domain.MetricEvidenceSet, error) {
	if !repository.available() || eventID <= 0 || windowEnd.IsZero() || !metricWindow(windowHours) {
		return domain.MetricEvidenceSet{}, sharedrepository.ErrUnavailable
	}
	windowEnd = windowEnd.UTC()
	windowStart := windowEnd.Add(-time.Duration(windowHours) * time.Hour)
	set := domain.MetricEvidenceSet{EventID: eventID, Evidence: []domain.MetricEvidence{}, Populations: []domain.MetricPopulation{}}
	if err := repository.metricQuery(ctx).QueryRowContext(ctx, `
SELECT first_seen_at, last_seen_at
FROM events
WHERE id = $1 AND deleted_at IS NULL`, eventID).Scan(&set.FirstSeenAt, &set.LastSeenAt); err != nil {
		if err == sql.ErrNoRows {
			return domain.MetricEvidenceSet{}, sharedrepository.ErrNotFound
		}
		return domain.MetricEvidenceSet{}, sharedrepository.MapError(err)
	}
	rows, err := repository.metricQuery(ctx).QueryContext(ctx, `
WITH eligible AS (
    SELECT c.id, c.source_connection_id, c.author_id, c.content_type, c.published_at
    FROM event_contents ec
    JOIN contents c ON c.id = ec.content_id
    WHERE ec.event_id = $1
      AND ec.evidence_role <> 'duplicate'
      AND c.content_status = 'active'
      AND c.deleted_at IS NULL
      AND c.published_at <= $2
), latest AS (
    SELECT DISTINCT ON (snapshot.content_id)
           snapshot.content_id, snapshot.captured_at, snapshot.view_count, snapshot.like_count, snapshot.comment_count, snapshot.share_count
    FROM content_metric_snapshots snapshot
    JOIN eligible ON eligible.id = snapshot.content_id
    WHERE snapshot.captured_at <= $2
    ORDER BY snapshot.content_id, snapshot.captured_at DESC
), baseline AS (
    SELECT DISTINCT ON (snapshot.content_id)
           snapshot.content_id, snapshot.captured_at, snapshot.view_count, snapshot.like_count, snapshot.comment_count, snapshot.share_count
    FROM content_metric_snapshots snapshot
    JOIN eligible ON eligible.id = snapshot.content_id
    WHERE snapshot.captured_at <= $3
    ORDER BY snapshot.content_id, snapshot.captured_at DESC
)
SELECT eligible.id, eligible.source_connection_id, eligible.author_id, eligible.content_type, eligible.published_at,
       baseline.captured_at, baseline.view_count, baseline.like_count, baseline.comment_count, baseline.share_count,
       latest.captured_at, latest.view_count, latest.like_count, latest.comment_count, latest.share_count
FROM eligible
LEFT JOIN baseline ON baseline.content_id = eligible.id
LEFT JOIN latest ON latest.content_id = eligible.id
ORDER BY eligible.id ASC`, eventID, windowEnd, windowStart)
	if err != nil {
		return domain.MetricEvidenceSet{}, sharedrepository.MapError(err)
	}
	defer rows.Close()
	keys := map[domain.MetricPopulationKey]struct{}{}
	for rows.Next() {
		item, err := scanMetricEvidence(rows)
		if err != nil {
			return domain.MetricEvidenceSet{}, err
		}
		set.Evidence = append(set.Evidence, item)
		keys[domain.MetricPopulationKey{SourceConnectionID: item.SourceConnectionID, ContentType: item.ContentType}] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return domain.MetricEvidenceSet{}, sharedrepository.MapError(err)
	}
	ordered := make([]domain.MetricPopulationKey, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].SourceConnectionID == ordered[right].SourceConnectionID {
			return ordered[left].ContentType < ordered[right].ContentType
		}
		return ordered[left].SourceConnectionID < ordered[right].SourceConnectionID
	})
	for _, key := range ordered {
		population, err := repository.loadMetricPopulation(ctx, key, windowEnd, windowStart)
		if err != nil {
			return domain.MetricEvidenceSet{}, err
		}
		set.Populations = append(set.Populations, population)
	}
	return set, nil
}

func (repository *Repository) ListHeatSnapshots(ctx context.Context, eventID int64, windowHours int, before time.Time, limit int) ([]domain.HeatResult, error) {
	if !repository.available() || eventID <= 0 || !metricWindow(windowHours) || before.IsZero() || limit < 1 || limit > 20 {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := repository.metricQuery(ctx).QueryContext(ctx, `
SELECT event_id, captured_at, window_hours, heat_score, trend_score, trend_status, source_count, content_count,
       heat_version, evidence_set_hash, capability_profile_set_hash
FROM event_metric_snapshots
WHERE event_id = $1 AND window_hours = $2 AND captured_at < $3
ORDER BY captured_at DESC, id DESC
LIMIT $4`, eventID, windowHours, before.UTC(), limit)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	results := make([]domain.HeatResult, 0, limit)
	for rows.Next() {
		var result domain.HeatResult
		if err := rows.Scan(&result.EventID, &result.WindowEnd, &result.WindowHours, &result.HeatScore, &result.TrendScore, &result.TrendStatus, &result.SourceCount, &result.ContentCount, &result.HeatVersion, &result.EvidenceSetHash, &result.CapabilityProfileSetHash); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	for left, right := 0, len(results)-1; left < right; left, right = left+1, right-1 {
		results[left], results[right] = results[right], results[left]
	}
	return results, nil
}

func (repository *Repository) SaveRecomputedHeatSnapshots(ctx context.Context, results []domain.HeatResult) error {
	if !repository.available() || len(results) == 0 {
		return sharedrepository.ErrUnavailable
	}
	ordered := append([]domain.HeatResult(nil), results...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].WindowHours < ordered[right].WindowHours })
	for index := range ordered {
		result := &ordered[index]
		if result.EventID <= 0 || result.WindowEnd.IsZero() || !metricWindow(result.WindowHours) || result.HeatVersion == "" || len(result.EvidenceSetHash) != 64 || len(result.CapabilityProfileSetHash) != 64 {
			return sharedrepository.ErrInvalidInput
		}
		if result.ReasonCodes == nil {
			result.ReasonCodes = []string{}
		}
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		for _, result := range ordered {
			if _, err := transaction.SQL.ExecContext(ctx, `
INSERT INTO event_metric_snapshots (event_id, captured_at, window_hours, heat_score, trend_score, trend_status, source_count, content_count, heat_version, evidence_set_hash, capability_profile_set_hash)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (event_id, captured_at, window_hours, heat_version, evidence_set_hash, capability_profile_set_hash) DO NOTHING`, result.EventID, result.WindowEnd.UTC(), result.WindowHours, result.HeatScore, result.TrendScore, string(result.TrendStatus), result.SourceCount, result.ContentCount, result.HeatVersion, result.EvidenceSetHash, result.CapabilityProfileSetHash); err != nil {
				return sharedrepository.MapError(err)
			}
		}
		current, found := currentHeatProjection(ordered)
		if !found {
			return sharedrepository.ErrInvalidInput
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE events
SET heat_score = $1, trend_score = $2, trend_status = $3, heat_window_hours = $4, heat_version = $5,
    heat_reason_codes = $6::text[], metric_capability_profile_set_hash = $7, heat_calculated_at = $8, updated_at = now()
WHERE id = $9 AND deleted_at IS NULL`, current.HeatScore, current.TrendScore, string(current.TrendStatus), current.WindowHours, current.HeatVersion, current.ReasonCodes, current.CapabilityProfileSetHash, current.WindowEnd.UTC(), current.EventID); err != nil {
			return sharedrepository.MapError(err)
		}
		return repository.updateMonitorMetricProjection(ctx, transaction.SQL, current)
	})
}

func (repository *Repository) loadMetricPopulation(ctx context.Context, key domain.MetricPopulationKey, windowEnd, windowStart time.Time) (domain.MetricPopulation, error) {
	rows, err := repository.metricQuery(ctx).QueryContext(ctx, `
WITH eligible AS (
    SELECT id FROM contents
    WHERE source_connection_id = $1 AND content_type = $2 AND content_status = 'active' AND deleted_at IS NULL
), latest AS (
    SELECT DISTINCT ON (snapshot.content_id)
           snapshot.content_id, snapshot.view_count, snapshot.like_count, snapshot.comment_count, snapshot.share_count
    FROM content_metric_snapshots snapshot
    JOIN eligible ON eligible.id = snapshot.content_id
    WHERE snapshot.captured_at <= $3
    ORDER BY snapshot.content_id, snapshot.captured_at DESC
), baseline AS (
    SELECT DISTINCT ON (snapshot.content_id)
           snapshot.content_id, snapshot.view_count, snapshot.like_count, snapshot.comment_count, snapshot.share_count
    FROM content_metric_snapshots snapshot
    JOIN eligible ON eligible.id = snapshot.content_id
    WHERE snapshot.captured_at <= $4
    ORDER BY snapshot.content_id, snapshot.captured_at DESC
)
SELECT latest.view_count - baseline.view_count, latest.like_count - baseline.like_count,
       latest.comment_count - baseline.comment_count, latest.share_count - baseline.share_count
FROM latest
JOIN baseline ON baseline.content_id = latest.content_id`, key.SourceConnectionID, key.ContentType, windowEnd, windowStart)
	if err != nil {
		return domain.MetricPopulation{}, sharedrepository.MapError(err)
	}
	defer rows.Close()
	population := domain.MetricPopulation{MetricPopulationKey: key, Deltas: []domain.MetricCounts{}}
	for rows.Next() {
		var views, likes, comments, shares sql.NullInt64
		if err := rows.Scan(&views, &likes, &comments, &shares); err != nil {
			return domain.MetricPopulation{}, sharedrepository.MapError(err)
		}
		population.Deltas = append(population.Deltas, domain.MetricCounts{Views: nullInt64Pointer(views), Likes: nullInt64Pointer(likes), Comments: nullInt64Pointer(comments), Shares: nullInt64Pointer(shares)})
	}
	if err := rows.Err(); err != nil {
		return domain.MetricPopulation{}, sharedrepository.MapError(err)
	}
	return population, nil
}

func (repository *Repository) updateMonitorMetricProjection(ctx context.Context, transaction *sql.Tx, current domain.HeatResult) error {
	rows, err := transaction.QueryContext(ctx, `
SELECT monitor_event.id, monitor_event.relevance_score,
       COALESCE(config.relevance_threshold, 100), event.last_seen_at
FROM monitor_events monitor_event
JOIN events event ON event.id = monitor_event.event_id
JOIN monitors monitor ON monitor.id = monitor_event.monitor_id
LEFT JOIN monitor_config_versions config ON config.id = monitor.published_config_version_id
WHERE monitor_event.event_id = $1
FOR UPDATE OF monitor_event`, current.EventID)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	defer rows.Close()
	type projection struct {
		id    int64
		final float64
	}
	updates := make([]projection, 0)
	for rows.Next() {
		var id int64
		var relevance, threshold float64
		var lastSeen time.Time
		if err := rows.Scan(&id, &relevance, &threshold, &lastSeen); err != nil {
			return sharedrepository.MapError(err)
		}
		freshness := current.WindowEnd.Sub(lastSeen).Hours()
		if freshness < 0 {
			freshness = 0
		}
		final, _, err := domain.RankMonitorEvent(domain.RankingInput{RelevanceScore: relevance, MinimumRelevance: threshold, HeatScore: current.HeatScore, TrendScore: current.TrendScore, FreshnessHours: freshness})
		if err != nil {
			return fmt.Errorf("rank monitor event: %w", err)
		}
		updates = append(updates, projection{id: id, final: final})
	}
	if err := rows.Err(); err != nil {
		return sharedrepository.MapError(err)
	}
	for _, update := range updates {
		if _, err := transaction.ExecContext(ctx, `UPDATE monitor_events SET final_score = $1, version = version + 1, updated_at = now() WHERE id = $2`, update.final, update.id); err != nil {
			return sharedrepository.MapError(err)
		}
	}
	return nil
}

func scanMetricEvidence(rows *sql.Rows) (domain.MetricEvidence, error) {
	var item domain.MetricEvidence
	var author sql.NullInt64
	var baselineAt, latestAt sql.NullTime
	var baselineViews, baselineLikes, baselineComments, baselineShares sql.NullInt64
	var latestViews, latestLikes, latestComments, latestShares sql.NullInt64
	if err := rows.Scan(&item.ContentID, &item.SourceConnectionID, &author, &item.ContentType, &item.PublishedAt,
		&baselineAt, &baselineViews, &baselineLikes, &baselineComments, &baselineShares,
		&latestAt, &latestViews, &latestLikes, &latestComments, &latestShares); err != nil {
		return domain.MetricEvidence{}, sharedrepository.MapError(err)
	}
	if author.Valid {
		value := author.Int64
		item.AuthorID = &value
	}
	if baselineAt.Valid {
		item.BaselineAt = baselineAt.Time
	}
	if latestAt.Valid {
		item.LatestAt = latestAt.Time
	}
	item.Baseline = domain.MetricCounts{Views: nullInt64Pointer(baselineViews), Likes: nullInt64Pointer(baselineLikes), Comments: nullInt64Pointer(baselineComments), Shares: nullInt64Pointer(baselineShares)}
	item.Latest = domain.MetricCounts{Views: nullInt64Pointer(latestViews), Likes: nullInt64Pointer(latestLikes), Comments: nullInt64Pointer(latestComments), Shares: nullInt64Pointer(latestShares)}
	return item, nil
}

func (repository *Repository) metricQuery(ctx context.Context) metricQuery {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func currentHeatProjection(results []domain.HeatResult) (domain.HeatResult, bool) {
	for _, result := range results {
		if result.WindowHours == 24 {
			return result, true
		}
	}
	return domain.HeatResult{}, false
}

func metricWindow(value int) bool { return value == 1 || value == 6 || value == 24 }

func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
