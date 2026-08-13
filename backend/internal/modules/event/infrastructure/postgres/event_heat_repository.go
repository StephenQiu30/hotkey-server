package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type EventHeatRepository struct{ runtime *database.Runtime }

type eventMetricEvidence struct {
	Evidence              eventdomain.MetricEvidence
	Capability            eventdomain.MetricCapability
	PopulationWindowHours int
}

func normalizeEventMetricEngagement(evidences []eventMetricEvidence, populations map[eventdomain.MetricPopulationKey]eventdomain.MetricPopulation) (*float64, bool, error) {
	var total float64
	available := 0
	usedFallback := false
	for _, item := range evidences {
		key := eventdomain.MetricPopulationKey{
			SourceConnectionID: item.Evidence.SourceConnectionID,
			ContentType:        item.Evidence.ContentType,
		}
		population, found := populations[key]
		if !found {
			population.MetricPopulationKey = key
		}
		normalized, fallback, err := eventdomain.NormalizeEngagement(item.Evidence, item.Capability, population)
		if err != nil {
			return nil, false, err
		}
		if normalized == nil {
			continue
		}
		total += *normalized / 100
		available++
		usedFallback = usedFallback || fallback
	}
	if available == 0 {
		return nil, false, nil
	}
	value := total / float64(available)
	return &value, usedFallback, nil
}

var _ eventapplication.EventHeatRepository = (*EventHeatRepository)(nil)

func NewEventHeatRepository(runtime *database.Runtime) (*EventHeatRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("event heat database runtime is required")
	}
	return &EventHeatRepository{runtime: runtime}, nil
}

func (repository *EventHeatRepository) ListMetricMicroEventIDsForContent(ctx context.Context, contentID int64) ([]int64, error) {
	if repository == nil || repository.runtime == nil || contentID <= 0 {
		return nil, eventapplication.ErrInvalidEventHeatContract
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT DISTINCT COALESCE(event.merged_into_micro_event_id,event.id) AS current_micro_event_id
FROM contents AS content
JOIN documents AS document
  ON document.source_connection_id=content.source_connection_id
 AND document.external_work_id=content.external_id
 AND document.document_state='active'
JOIN content_family_members AS family_member
  ON family_member.document_version_id=document.current_document_version_id
 AND family_member.active
JOIN micro_event_members AS event_member
  ON event_member.content_family_id=family_member.family_id
 AND event_member.active
JOIN micro_events AS event ON event.id=event_member.micro_event_id
WHERE content.id=$1 AND content.content_status='active' AND content.deleted_at IS NULL
  AND event.status IN ('active','review_pending','closed','merged')
ORDER BY current_micro_event_id`, contentID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	result := make([]int64, 0)
	for rows.Next() {
		var microEventID int64
		if err := rows.Scan(&microEventID); err != nil {
			return nil, databaserepository.MapError(err)
		}
		result = append(result, microEventID)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return result, nil
}

func (repository *EventHeatRepository) ReadEventHeatTarget(ctx context.Context, query eventapplication.ReadEventHeatTargetQuery) (eventapplication.EventHeatTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.MicroEventID <= 0 ||
		(query.WindowHours != 1 && query.WindowHours != 6 && query.WindowHours != 24) || query.WindowEndedAt.IsZero() {
		return eventapplication.EventHeatTargetDTO{}, eventapplication.ErrInvalidEventHeatContract
	}
	windowEnd := query.WindowEndedAt.UTC()
	windowStart := windowEnd.Add(-time.Duration(query.WindowHours) * time.Hour)
	previousStart := windowStart.Add(-time.Duration(query.WindowHours) * time.Hour)
	priorStart := previousStart.Add(-time.Duration(query.WindowHours) * time.Hour)
	var value eventapplication.EventHeatTargetDTO
	err := repository.queryRow(ctx, `
WITH event_documents AS (
  SELECT DISTINCT member.content_family_id,version.id AS document_version_id,observation.id AS observation_id,
         COALESCE(observation.published_at,observation.captured_at) AS observed_at,
	         observation.source_connection_id,connection.source_type,
	         party.source_party_id
  FROM micro_event_members AS member
  JOIN content_family_members AS family_member ON family_member.family_id=member.content_family_id AND family_member.active
  JOIN content_fingerprints AS fingerprint ON fingerprint.id=family_member.fingerprint_id
    AND fingerprint.lifecycle_state='active' AND fingerprint.retention_until>$5
  JOIN document_versions AS version ON version.id=family_member.document_version_id
  JOIN source_observations AS observation ON observation.id=version.source_observation_id
  JOIN source_connections AS connection ON connection.id=observation.source_connection_id
  LEFT JOIN source_observation_parties AS party ON party.source_observation_id=observation.id AND party.role='publisher'
	  WHERE member.micro_event_id=$1 AND member.active
    AND current_rights_action_allowed(fingerprint.store_derived_rights_decision_id,fingerprint.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'store_derived',$5)
    AND current_rights_action_allowed(fingerprint.retain_rights_decision_id,fingerprint.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'retain',$5)
)
SELECT event.id,event.version,profile.id,profile.profile_version,
       profile.lineage_weight::float8,profile.velocity_weight::float8,profile.acceleration_weight::float8,
       profile.coverage_weight::float8,profile.engagement_weight::float8,profile.recency_weight::float8,
       $2::timestamptz,$5::timestamptz,
       count(DISTINCT document.content_family_id)::integer,
       count(DISTINCT document.observation_id) FILTER (WHERE document.observed_at>$2 AND document.observed_at<=$5)::integer,
       count(DISTINCT document.observation_id) FILTER (WHERE document.observed_at>$3 AND document.observed_at<=$2)::integer,
       count(DISTINCT document.observation_id) FILTER (WHERE document.observed_at>$4 AND document.observed_at<=$3)::integer,
       count(DISTINCT document.source_party_id) FILTER (WHERE document.source_party_id IS NOT NULL)::integer,
	       count(DISTINCT document.source_type)::integer,
	       GREATEST(0,extract(epoch FROM ($5::timestamptz-COALESCE(max(document.observed_at),event.event_started_at)))/3600)::float8
FROM micro_events AS event CROSS JOIN event_heat_profiles AS profile
LEFT JOIN event_documents AS document ON true
WHERE event.id=$1 AND event.status IN ('active','review_pending','closed') AND profile.status='active'
GROUP BY event.id,event.version,event.event_started_at,profile.id,profile.profile_version,
 profile.lineage_weight,profile.velocity_weight,profile.acceleration_weight,profile.coverage_weight,
 profile.engagement_weight,profile.recency_weight`, query.MicroEventID, windowStart, previousStart, priorStart,
		windowEnd).Scan(&value.MicroEventID, &value.MicroEventVersion, &value.HeatProfileID,
		&value.HeatProfileVersion, &value.Weights.Lineage, &value.Weights.Velocity, &value.Weights.Acceleration,
		&value.Weights.Coverage, &value.Weights.Engagement, &value.Weights.Recency, &value.WindowStartedAt,
		&value.WindowEndedAt, &value.IndependentLineageRoots, &value.ReportsInWindow,
		&value.ReportsInPreviousWindow, &value.ReportsInPriorWindow, &value.PublisherCoverage,
		&value.SourceTypeCoverage, &value.AgeHours)
	if errors.Is(err, sql.ErrNoRows) {
		return eventapplication.EventHeatTargetDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return eventapplication.EventHeatTargetDTO{}, databaserepository.MapError(err)
	}
	value.NormalizedEngagement, value.NormalizationFallback, err = repository.readNormalizedEngagement(
		ctx, query.MicroEventID, windowStart, windowEnd,
	)
	if err != nil {
		return eventapplication.EventHeatTargetDTO{}, err
	}
	return value, nil
}

func (repository *EventHeatRepository) readNormalizedEngagement(ctx context.Context, microEventID int64, windowStart, windowEnd time.Time) (*float64, bool, error) {
	evidences, err := repository.readEventMetricEvidence(ctx, microEventID, windowStart, windowEnd)
	if err != nil || len(evidences) == 0 {
		return nil, false, err
	}
	populations := make(map[eventdomain.MetricPopulationKey]eventdomain.MetricPopulation)
	for _, item := range evidences {
		key := eventdomain.MetricPopulationKey{
			SourceConnectionID: item.Evidence.SourceConnectionID,
			ContentType:        item.Evidence.ContentType,
		}
		if _, found := populations[key]; found {
			continue
		}
		population, err := repository.readMetricPopulation(ctx, key, windowStart, windowEnd, item.PopulationWindowHours)
		if err != nil {
			return nil, false, err
		}
		populations[key] = population
	}
	engagement, fallback, err := normalizeEventMetricEngagement(evidences, populations)
	if err != nil {
		return nil, false, fmt.Errorf("normalize micro-event engagement: %w", err)
	}
	return engagement, fallback, nil
}

func (repository *EventHeatRepository) readEventMetricEvidence(ctx context.Context, microEventID int64, windowStart, windowEnd time.Time) ([]eventMetricEvidence, error) {
	rows, err := repository.queryRows(ctx, `
WITH event_content AS (
  SELECT DISTINCT content.id,content.source_connection_id,content.content_type,content.published_at,
         connection.source_type,profile.id AS profile_id,profile.version AS profile_record_version,
         profile.profile_version,profile.supports_views,profile.supports_likes,
         profile.supports_comments,profile.supports_shares,profile.independence_strategy,
         profile.normalization_window_hours,profile.credibility_weight::float8,
         profile.max_single_item_contribution::float8
  FROM micro_event_members AS event_member
  JOIN content_family_members AS family_member
    ON family_member.family_id=event_member.content_family_id AND family_member.active
  JOIN content_fingerprints AS fingerprint
    ON fingerprint.id=family_member.fingerprint_id
   AND fingerprint.lifecycle_state='active' AND fingerprint.retention_until>$3
  JOIN document_versions AS version ON version.id=family_member.document_version_id
  JOIN documents AS document ON document.id=version.document_id
  JOIN contents AS content
    ON content.source_connection_id=document.source_connection_id
   AND content.external_id=document.external_work_id
   AND content.content_status='active' AND content.deleted_at IS NULL
  JOIN source_connections AS connection ON connection.id=content.source_connection_id
  JOIN metric_capability_profiles AS profile
    ON profile.source_type=connection.source_type AND profile.status='published'
  WHERE event_member.micro_event_id=$1 AND event_member.active
    AND current_rights_action_allowed(fingerprint.store_derived_rights_decision_id,fingerprint.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'store_derived',$3)
    AND current_rights_action_allowed(fingerprint.retain_rights_decision_id,fingerprint.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'retain',$3)
)
SELECT content.id,content.source_connection_id,content.content_type,content.published_at,content.source_type,
       content.profile_id,content.profile_record_version,content.profile_version,
       content.supports_views,content.supports_likes,content.supports_comments,content.supports_shares,
       content.independence_strategy,content.normalization_window_hours,content.credibility_weight,
       content.max_single_item_contribution,
       baseline.captured_at,baseline.view_count,baseline.like_count,baseline.comment_count,baseline.share_count,
       latest.captured_at,latest.view_count,latest.like_count,latest.comment_count,latest.share_count
FROM event_content AS content
LEFT JOIN LATERAL (
  SELECT snapshot.captured_at,snapshot.view_count,snapshot.like_count,snapshot.comment_count,snapshot.share_count
  FROM content_metric_snapshots AS snapshot
  WHERE snapshot.content_id=content.id AND snapshot.captured_at<=$2
  ORDER BY snapshot.captured_at DESC,snapshot.id DESC LIMIT 1
) AS baseline ON true
LEFT JOIN LATERAL (
  SELECT snapshot.captured_at,snapshot.view_count,snapshot.like_count,snapshot.comment_count,snapshot.share_count
  FROM content_metric_snapshots AS snapshot
  WHERE snapshot.content_id=content.id AND snapshot.captured_at<=$3
  ORDER BY snapshot.captured_at DESC,snapshot.id DESC LIMIT 1
) AS latest ON true
ORDER BY content.id`, microEventID, windowStart.UTC(), windowEnd.UTC())
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	result := make([]eventMetricEvidence, 0)
	for rows.Next() {
		var item eventMetricEvidence
		var baselineAt, latestAt sql.NullTime
		var baselineViews, baselineLikes, baselineComments, baselineShares sql.NullInt64
		var latestViews, latestLikes, latestComments, latestShares sql.NullInt64
		if err := rows.Scan(
			&item.Evidence.ContentID, &item.Evidence.SourceConnectionID, &item.Evidence.ContentType,
			&item.Evidence.PublishedAt, &item.Capability.SourceType, &item.Capability.ProfileID,
			&item.Capability.ProfileRecordVer, &item.Capability.ProfileVersion,
			&item.Capability.SupportsViews, &item.Capability.SupportsLikes, &item.Capability.SupportsComments,
			&item.Capability.SupportsShares, &item.Capability.IndependenceStrategy,
			&item.PopulationWindowHours, &item.Capability.CredibilityWeight,
			&item.Capability.MaxSingleItemContribution,
			&baselineAt, &baselineViews, &baselineLikes, &baselineComments, &baselineShares,
			&latestAt, &latestViews, &latestLikes, &latestComments, &latestShares,
		); err != nil {
			return nil, databaserepository.MapError(err)
		}
		item.Evidence.BaselineAt, item.Evidence.LatestAt = nullableMetricTime(baselineAt), nullableMetricTime(latestAt)
		item.Evidence.Baseline = metricCounts(baselineViews, baselineLikes, baselineComments, baselineShares)
		item.Evidence.Latest = metricCounts(latestViews, latestLikes, latestComments, latestShares)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return result, nil
}

func (repository *EventHeatRepository) readMetricPopulation(ctx context.Context, key eventdomain.MetricPopulationKey, windowStart, windowEnd time.Time, windowHours int) (eventdomain.MetricPopulation, error) {
	population := eventdomain.MetricPopulation{MetricPopulationKey: key, Deltas: []eventdomain.MetricCounts{}}
	rows, err := repository.queryRows(ctx, `
SELECT baseline.view_count,baseline.like_count,baseline.comment_count,baseline.share_count,
       latest.view_count,latest.like_count,latest.comment_count,latest.share_count
FROM contents AS content
JOIN LATERAL (
  SELECT snapshot.captured_at,snapshot.view_count,snapshot.like_count,snapshot.comment_count,snapshot.share_count
  FROM content_metric_snapshots AS snapshot
  WHERE snapshot.content_id=content.id AND snapshot.captured_at<=$4
  ORDER BY snapshot.captured_at DESC,snapshot.id DESC LIMIT 1
) AS latest ON true
LEFT JOIN LATERAL (
  SELECT snapshot.view_count,snapshot.like_count,snapshot.comment_count,snapshot.share_count
  FROM content_metric_snapshots AS snapshot
  WHERE snapshot.content_id=content.id AND snapshot.captured_at<=$3
  ORDER BY snapshot.captured_at DESC,snapshot.id DESC LIMIT 1
) AS baseline ON true
WHERE content.source_connection_id=$1 AND content.content_type=$2
  AND content.content_status='active' AND content.deleted_at IS NULL
  AND latest.captured_at>$4-($5 * interval '1 hour')
ORDER BY content.id`, key.SourceConnectionID, key.ContentType, windowStart.UTC(), windowEnd.UTC(), windowHours)
	if err != nil {
		return population, databaserepository.MapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var baselineViews, baselineLikes, baselineComments, baselineShares sql.NullInt64
		var latestViews, latestLikes, latestComments, latestShares sql.NullInt64
		if err := rows.Scan(&baselineViews, &baselineLikes, &baselineComments, &baselineShares,
			&latestViews, &latestLikes, &latestComments, &latestShares); err != nil {
			return population, databaserepository.MapError(err)
		}
		population.Deltas = append(population.Deltas, eventdomain.MetricCounts{
			Views: metricDeltaPointer(latestViews, baselineViews), Likes: metricDeltaPointer(latestLikes, baselineLikes),
			Comments: metricDeltaPointer(latestComments, baselineComments), Shares: metricDeltaPointer(latestShares, baselineShares),
		})
	}
	if err := rows.Err(); err != nil {
		return population, databaserepository.MapError(err)
	}
	return population, nil
}

func metricCounts(views, likes, comments, shares sql.NullInt64) eventdomain.MetricCounts {
	return eventdomain.MetricCounts{Views: nullableMetric(views), Likes: nullableMetric(likes), Comments: nullableMetric(comments), Shares: nullableMetric(shares)}
}

func nullableMetric(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullableMetricTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func metricDeltaPointer(latest, baseline sql.NullInt64) *int64 {
	if !latest.Valid || !baseline.Valid || latest.Int64 < baseline.Int64 {
		return nil
	}
	value := latest.Int64 - baseline.Int64
	return &value
}

func (repository *EventHeatRepository) CommitEventHeatSnapshot(ctx context.Context, command eventapplication.CommitEventHeatSnapshotCommand) (eventapplication.EventHeatSnapshotDTO, error) {
	if repository == nil || repository.runtime == nil || !validEventHeatCommit(command) {
		return eventapplication.EventHeatSnapshotDTO{}, eventapplication.ErrInvalidEventHeatContract
	}
	reasons, _ := json.Marshal(append([]string{}, command.ReasonCodes...))
	var engagement any
	if command.NormalizedEngagement != nil {
		engagement = *command.NormalizedEngagement
	}
	var record eventHeatSnapshotRecord
	err := repository.queryRow(ctx, `
INSERT INTO micro_event_heat_snapshots (
 micro_event_id,micro_event_version,heat_profile_id,window_started_at,window_ended_at,
 independent_lineage_root_count,velocity,acceleration,coverage,normalized_engagement,recency,
 available_weight,heat_score,reason_codes
) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb
WHERE EXISTS (SELECT 1 FROM micro_events WHERE id=$1 AND version=$2)
  AND EXISTS (SELECT 1 FROM event_heat_profiles WHERE id=$3 AND status='active')
ON CONFLICT (micro_event_id,micro_event_version,heat_profile_id,window_started_at,window_ended_at) DO NOTHING
RETURNING id,micro_event_id,micro_event_version,heat_profile_id,window_started_at,window_ended_at,
 independent_lineage_root_count,velocity::float8,acceleration::float8,coverage::float8,
 normalized_engagement::float8,recency::float8,available_weight::float8,heat_score::float8,reason_codes`,
		command.MicroEventID, command.MicroEventVersion, command.HeatProfileID, command.WindowStartedAt.UTC(),
		command.WindowEndedAt.UTC(), command.IndependentLineageRoots, command.Velocity, command.Acceleration,
		command.Coverage, engagement, command.Recency, command.AvailableWeight, command.HeatScore, string(reasons)).Scan(record.scanDestinations()...)
	if err == nil {
		value, mapErr := record.dto(true)
		return value, mapErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return eventapplication.EventHeatSnapshotDTO{}, databaserepository.MapError(err)
	}
	err = repository.queryRow(ctx, `
SELECT id,micro_event_id,micro_event_version,heat_profile_id,window_started_at,window_ended_at,
 independent_lineage_root_count,velocity::float8,acceleration::float8,coverage::float8,
 normalized_engagement::float8,recency::float8,available_weight::float8,heat_score::float8,reason_codes
FROM micro_event_heat_snapshots
WHERE micro_event_id=$1 AND micro_event_version=$2 AND heat_profile_id=$3 AND window_started_at=$4 AND window_ended_at=$5`,
		command.MicroEventID, command.MicroEventVersion, command.HeatProfileID, command.WindowStartedAt.UTC(), command.WindowEndedAt.UTC()).Scan(record.scanDestinations()...)
	if err != nil {
		return eventapplication.EventHeatSnapshotDTO{}, databaserepository.MapError(err)
	}
	value, err := record.dto(false)
	if err != nil || !eventHeatSnapshotMatchesCommand(value, command) {
		return eventapplication.EventHeatSnapshotDTO{}, fmt.Errorf("event heat snapshot conflict")
	}
	return value, nil
}

type eventHeatSnapshotRecord struct {
	id, microEventID, microEventVersion, heatProfileID int64
	windowStartedAt, windowEndedAt                     time.Time
	lineageRoots                                       int
	velocity, acceleration, coverage                   float64
	engagement                                         sql.NullFloat64
	recency, availableWeight, heatScore                float64
	reasonsJSON                                        []byte
}

func (record *eventHeatSnapshotRecord) scanDestinations() []any {
	return []any{&record.id, &record.microEventID, &record.microEventVersion, &record.heatProfileID,
		&record.windowStartedAt, &record.windowEndedAt, &record.lineageRoots, &record.velocity, &record.acceleration,
		&record.coverage, &record.engagement, &record.recency, &record.availableWeight, &record.heatScore, &record.reasonsJSON}
}

func (record eventHeatSnapshotRecord) dto(created bool) (eventapplication.EventHeatSnapshotDTO, error) {
	reasons := []string{}
	if err := json.Unmarshal(record.reasonsJSON, &reasons); err != nil {
		return eventapplication.EventHeatSnapshotDTO{}, err
	}
	value := eventapplication.EventHeatSnapshotDTO{ID: record.id, MicroEventID: record.microEventID,
		MicroEventVersion: record.microEventVersion, HeatProfileID: record.heatProfileID,
		WindowStartedAt: record.windowStartedAt.UTC(), WindowEndedAt: record.windowEndedAt.UTC(),
		IndependentLineageRoots: record.lineageRoots, Velocity: record.velocity, Acceleration: record.acceleration,
		Coverage: record.coverage, Recency: record.recency, AvailableWeight: record.availableWeight,
		HeatScore: record.heatScore, ReasonCodes: reasons, Created: created}
	if record.engagement.Valid {
		value.NormalizedEngagement = floatPointer(record.engagement.Float64)
	}
	return value, nil
}

func validEventHeatCommit(value eventapplication.CommitEventHeatSnapshotCommand) bool {
	return value.MicroEventID > 0 && value.MicroEventVersion > 0 && value.HeatProfileID > 0 &&
		!value.WindowStartedAt.IsZero() && value.WindowEndedAt.After(value.WindowStartedAt) && value.IndependentLineageRoots >= 0 &&
		validUnitHeat(value.Velocity) && validUnitHeat(value.Acceleration) && validUnitHeat(value.Coverage) &&
		(value.NormalizedEngagement == nil || validUnitHeat(*value.NormalizedEngagement)) && validUnitHeat(value.Recency) &&
		value.AvailableWeight > 0 && value.AvailableWeight <= 1 && !math.IsNaN(value.HeatScore) && !math.IsInf(value.HeatScore, 0) &&
		value.HeatScore >= 0 && value.HeatScore <= 100
}

func validUnitHeat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func eventHeatSnapshotMatchesCommand(value eventapplication.EventHeatSnapshotDTO, command eventapplication.CommitEventHeatSnapshotCommand) bool {
	return value.MicroEventID == command.MicroEventID && value.MicroEventVersion == command.MicroEventVersion &&
		value.HeatProfileID == command.HeatProfileID && value.WindowStartedAt.Equal(command.WindowStartedAt) &&
		value.WindowEndedAt.Equal(command.WindowEndedAt) && value.IndependentLineageRoots == command.IndependentLineageRoots &&
		value.Velocity == command.Velocity && value.Acceleration == command.Acceleration && value.Coverage == command.Coverage &&
		optionalHeatEquals(value.NormalizedEngagement, command.NormalizedEngagement) && value.Recency == command.Recency &&
		value.AvailableWeight == command.AvailableWeight && value.HeatScore == command.HeatScore
}

func optionalHeatEquals(left, right *float64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
func floatPointer(value float64) *float64 { return &value }

func (repository *EventHeatRepository) queryRow(ctx context.Context, query string, arguments ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, arguments...)
	}
	return repository.runtime.SQL.QueryRowContext(ctx, query, arguments...)
}

func (repository *EventHeatRepository) queryRows(ctx context.Context, query string, arguments ...any) (*sql.Rows, error) {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryContext(ctx, query, arguments...)
	}
	return repository.runtime.SQL.QueryContext(ctx, query, arguments...)
}
