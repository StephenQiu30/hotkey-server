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
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type EventHeatRepository struct{ runtime *database.Runtime }

var _ eventapplication.EventHeatRepository = (*EventHeatRepository)(nil)

func NewEventHeatRepository(runtime *database.Runtime) (*EventHeatRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("event heat database runtime is required")
	}
	return &EventHeatRepository{runtime: runtime}, nil
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
	var engagement sql.NullFloat64
	err := repository.queryRow(ctx, `
WITH event_documents AS (
  SELECT DISTINCT member.content_family_id,version.id AS document_version_id,observation.id AS observation_id,
         COALESCE(observation.published_at,observation.captured_at) AS observed_at,
         observation.source_connection_id,connection.source_type,
         party.source_party_id,run_item.content_id
  FROM micro_event_members AS member
  JOIN content_family_members AS family_member ON family_member.family_id=member.content_family_id AND family_member.active
  JOIN content_fingerprints AS fingerprint ON fingerprint.id=family_member.fingerprint_id
    AND fingerprint.lifecycle_state='active' AND fingerprint.retention_until>$5
  JOIN document_versions AS version ON version.id=family_member.document_version_id
  JOIN source_observations AS observation ON observation.id=version.source_observation_id
  JOIN source_connections AS connection ON connection.id=observation.source_connection_id
  LEFT JOIN source_observation_parties AS party ON party.source_observation_id=observation.id AND party.role='publisher'
  LEFT JOIN collection_run_items AS run_item ON run_item.id=observation.collection_run_item_id
  WHERE member.micro_event_id=$1 AND member.active
    AND current_rights_action_allowed(fingerprint.store_derived_rights_decision_id,fingerprint.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'store_derived',$5)
    AND current_rights_action_allowed(fingerprint.retain_rights_decision_id,fingerprint.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'retain',$5)
), latest_metrics AS (
  SELECT DISTINCT ON (snapshot.content_id) snapshot.content_id,
         COALESCE(snapshot.view_count,0)+COALESCE(snapshot.like_count,0)+COALESCE(snapshot.comment_count,0)+COALESCE(snapshot.share_count,0) AS engagement
  FROM content_metric_snapshots AS snapshot WHERE snapshot.captured_at<=$5
  ORDER BY snapshot.content_id,snapshot.captured_at DESC
), normalized_metrics AS (
  SELECT document.observation_id,CASE WHEN metric.engagement IS NULL THEN NULL
         ELSE metric.engagement::float8/NULLIF(max(metric.engagement) OVER (PARTITION BY document.source_connection_id),0) END AS normalized
  FROM event_documents AS document LEFT JOIN latest_metrics AS metric ON metric.content_id=document.content_id
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
       avg(metric.normalized) FILTER (WHERE metric.normalized IS NOT NULL)::float8,
       GREATEST(0,extract(epoch FROM ($5::timestamptz-COALESCE(max(document.observed_at),event.event_started_at)))/3600)::float8
FROM micro_events AS event CROSS JOIN event_heat_profiles AS profile
LEFT JOIN event_documents AS document ON true
LEFT JOIN normalized_metrics AS metric ON metric.observation_id=document.observation_id
WHERE event.id=$1 AND event.status IN ('active','review_pending','closed') AND profile.status='active'
GROUP BY event.id,event.version,event.event_started_at,profile.id,profile.profile_version,
 profile.lineage_weight,profile.velocity_weight,profile.acceleration_weight,profile.coverage_weight,
 profile.engagement_weight,profile.recency_weight`, query.MicroEventID, windowStart, previousStart, priorStart,
		windowEnd).Scan(&value.MicroEventID, &value.MicroEventVersion, &value.HeatProfileID,
		&value.HeatProfileVersion, &value.Weights.Lineage, &value.Weights.Velocity, &value.Weights.Acceleration,
		&value.Weights.Coverage, &value.Weights.Engagement, &value.Weights.Recency, &value.WindowStartedAt,
		&value.WindowEndedAt, &value.IndependentLineageRoots, &value.ReportsInWindow,
		&value.ReportsInPreviousWindow, &value.ReportsInPriorWindow, &value.PublisherCoverage,
		&value.SourceTypeCoverage, &engagement, &value.AgeHours)
	if errors.Is(err, sql.ErrNoRows) {
		return eventapplication.EventHeatTargetDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return eventapplication.EventHeatTargetDTO{}, databaserepository.MapError(err)
	}
	if engagement.Valid {
		value.NormalizedEngagement = floatPointer(engagement.Float64)
	}
	return value, nil
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
