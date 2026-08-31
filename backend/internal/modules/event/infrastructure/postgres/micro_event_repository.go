package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type MicroEventRepository struct{ runtime *database.Runtime }

var _ eventapplication.MicroEventRepository = (*MicroEventRepository)(nil)
var _ eventapplication.AcceptedMatchFamilyReader = (*MicroEventRepository)(nil)

func NewMicroEventRepository(runtime *database.Runtime) (*MicroEventRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("micro-event database runtime is required")
	}
	return &MicroEventRepository{runtime: runtime}, nil
}

func (repository *MicroEventRepository) ResolveAcceptedMatchFamily(ctx context.Context, query eventapplication.ResolveAcceptedMatchFamilyQuery) (eventapplication.AcceptedMatchFamilyDTO, error) {
	if repository == nil || repository.runtime == nil || query.DocumentMatchDecisionID <= 0 || query.DocumentVersionID <= 0 {
		return eventapplication.AcceptedMatchFamilyDTO{}, eventapplication.ErrInvalidAcceptedMatchProjectionContract
	}
	var value eventapplication.AcceptedMatchFamilyDTO
	err := repository.queryRow(ctx, `SELECT member.family_id,match.id,match.document_version_id,
COALESCE((SELECT override.decision FROM document_match_overrides AS override
          WHERE override.match_decision_id=match.id ORDER BY override.sequence_no DESC LIMIT 1),match.decision)
FROM document_match_decisions AS match
JOIN content_family_members AS member ON member.document_version_id=match.document_version_id AND member.active
JOIN content_families AS family ON family.id=member.family_id AND family.status IN ('active','review_pending')
WHERE match.id=$1 AND match.document_version_id=$2`, query.DocumentMatchDecisionID, query.DocumentVersionID).Scan(
		&value.ContentFamilyID, &value.DocumentMatchDecisionID, &value.DocumentVersionID, &value.EffectiveDecision)
	if errors.Is(err, sql.ErrNoRows) {
		return eventapplication.AcceptedMatchFamilyDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return eventapplication.AcceptedMatchFamilyDTO{}, databaserepository.MapError(err)
	}
	if value.EffectiveDecision != "accepted" {
		return eventapplication.AcceptedMatchFamilyDTO{}, sharedrepository.ErrConflict
	}
	return value, nil
}

func (repository *MicroEventRepository) ReadMicroEventAssignmentTarget(ctx context.Context, query eventapplication.ReadMicroEventAssignmentTargetQuery) (eventapplication.MicroEventAssignmentTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.ContentFamilyID <= 0 || query.DocumentMatchDecisionID <= 0 ||
		query.ClusteringProfileVersion != eventapplication.CanonicalMicroEventClusteringProfileVersion {
		return eventapplication.MicroEventAssignmentTargetDTO{}, eventapplication.ErrInvalidMicroEventContract
	}
	now := time.Now().UTC()
	var target eventapplication.MicroEventAssignmentTargetDTO
	var locations, identifiers []string
	err := repository.queryRow(ctx, `
SELECT family.id,match.id,match.document_version_id,match.monitor_id,match.monitor_version_id,
       COALESCE((SELECT override.decision FROM document_match_overrides AS override
                 WHERE override.match_decision_id=match.id ORDER BY override.sequence_no DESC LIMIT 1),match.decision),
       primary_subject.entity_key,search.action_keys[1],to_json(search.location_keys),'[]'::json,
       COALESCE(observation.published_at,observation.captured_at)
FROM document_match_decisions AS match
JOIN content_family_members AS member ON member.document_version_id=match.document_version_id
  AND member.family_id=$1 AND member.active
JOIN content_families AS family ON family.id=member.family_id AND family.status IN ('active','review_pending')
JOIN document_versions AS version ON version.id=match.document_version_id
JOIN source_observations AS observation ON observation.id=version.source_observation_id
JOIN document_version_search_indexes AS search ON search.document_version_id=version.id
  AND search.lifecycle_state='active' AND search.retention_until>$3
  AND cardinality(search.entity_keys)>0 AND cardinality(search.action_keys)>0
  AND current_rights_action_allowed(search.store_derived_rights_decision_id,search.source_connection_id,
      'document_version',version.id::text,version.content_sha256,'store_derived',$3)
  AND current_rights_action_allowed(search.retain_rights_decision_id,search.source_connection_id,
      'document_version',version.id::text,version.content_sha256,'retain',$3)
JOIN LATERAL (
  SELECT candidate.entity_key
  FROM unnest(search.entity_keys) AS candidate(entity_key)
  WHERE length(candidate.entity_key)>1 AND candidate.entity_key !~ '^[0-9]+$'
  ORDER BY EXISTS (
             SELECT 1
             FROM monitor_compiled_entities AS compiled_entity
             JOIN monitor_compiled_entity_aliases AS alias ON alias.compiled_entity_id=compiled_entity.id
             WHERE compiled_entity.compiled_profile_id=match.compiled_profile_id
               AND alias.normalized_alias=candidate.entity_key
           ) DESC,
           EXISTS (
             SELECT 1 FROM monitor_compiled_clauses AS clause
             WHERE clause.compiled_profile_id=match.compiled_profile_id
               AND clause.operator IN ('must','should')
               AND clause.normalized_value=candidate.entity_key
           ) DESC,
           length(candidate.entity_key),candidate.entity_key
  LIMIT 1
) AS primary_subject ON true
WHERE match.id=$2`, query.ContentFamilyID, query.DocumentMatchDecisionID, now).Scan(&target.ContentFamilyID,
		&target.DocumentMatchDecisionID, &target.DocumentVersionID, &target.MonitorID, &target.MonitorVersionID, &target.EffectiveMatchDecision,
		&target.PrimarySubjectKey, &target.PrimaryActionKey, microEventStringArrayScan{destination: &locations},
		microEventStringArrayScan{destination: &identifiers}, &target.OccurredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return eventapplication.MicroEventAssignmentTargetDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return eventapplication.MicroEventAssignmentTargetDTO{}, databaserepository.MapError(err)
	}
	target.LocationKeys = append([]string(nil), locations...)
	target.IdentifierKeys = append([]string(nil), identifiers...)
	existingKey := microEventAssignmentKey(target.ContentFamilyID, query.ClusteringProfileVersion)
	if stored, found, readErr := readMicroEventDecision(ctx, repository.queryExecutor(ctx), existingKey); readErr != nil {
		return eventapplication.MicroEventAssignmentTargetDTO{}, readErr
	} else if found {
		value, conversionErr := microEventResultFromRecords(stored.Event, stored.Decision)
		if conversionErr != nil {
			return eventapplication.MicroEventAssignmentTargetDTO{}, conversionErr
		}
		target.ExistingAssignment = &value
		return target, nil
	}
	candidates, err := repository.readCandidates(ctx, target)
	if err != nil {
		return eventapplication.MicroEventAssignmentTargetDTO{}, err
	}
	target.Candidates = candidates
	return target, nil
}

func microEventAssignmentKey(contentFamilyID int64, profile string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("micro-event:%d:%s", contentFamilyID, profile)))
	return "micro-event-" + hex.EncodeToString(digest[:16])
}

func (repository *MicroEventRepository) readCandidates(ctx context.Context, target eventapplication.MicroEventAssignmentTargetDTO) ([]eventapplication.MicroEventCandidateDTO, error) {
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, `
WITH target_embedding AS MATERIALIZED (
  SELECT embedding.embedding,embedding.model_profile_id,embedding.model_profile_version,embedding.model_version
  FROM document_version_embeddings AS embedding
  JOIN document_versions AS version ON version.id=embedding.document_version_id
  WHERE embedding.document_version_id=$6 AND embedding.lifecycle_state='active' AND embedding.retention_until>CURRENT_TIMESTAMP
    AND current_rights_action_allowed(embedding.embed_local_rights_decision_id,embedding.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'embed_local',CURRENT_TIMESTAMP)
    AND current_rights_action_allowed(embedding.retain_rights_decision_id,embedding.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'retain',CURRENT_TIMESTAMP)
  ORDER BY embedding.created_at DESC,embedding.id DESC LIMIT 1
), ann_documents AS MATERIALIZED (
  SELECT candidate.document_version_id,
         GREATEST(0,LEAST(1,1-(candidate.embedding <=> target.embedding)))::float8 AS dense_similarity
  FROM target_embedding AS target
  CROSS JOIN LATERAL (
    SELECT embedding.*
    FROM document_version_embeddings AS embedding
    JOIN document_versions AS version ON version.id=embedding.document_version_id
    WHERE embedding.document_version_id<>$6 AND embedding.lifecycle_state='active'
      AND embedding.model_profile_id=target.model_profile_id
      AND embedding.model_profile_version=target.model_profile_version
      AND embedding.model_version=target.model_version
      AND embedding.retention_until>CURRENT_TIMESTAMP
      AND current_rights_action_allowed(embedding.embed_local_rights_decision_id,embedding.source_connection_id,
          'document_version',version.id::text,version.content_sha256,'embed_local',CURRENT_TIMESTAMP)
      AND current_rights_action_allowed(embedding.retain_rights_decision_id,embedding.source_connection_id,
          'document_version',version.id::text,version.content_sha256,'retain',CURRENT_TIMESTAMP)
    ORDER BY embedding.embedding <=> target.embedding LIMIT 100
  ) AS candidate
), ann_events AS MATERIALIZED (
  SELECT member.micro_event_id,max(document.dense_similarity)::float8 AS dense_similarity
  FROM ann_documents AS document
  JOIN content_family_members AS family_member ON family_member.document_version_id=document.document_version_id AND family_member.active
  JOIN micro_event_members AS member ON member.content_family_id=family_member.family_id AND member.active
  GROUP BY member.micro_event_id
), candidate_events AS MATERIALIZED (
  SELECT event.id
  FROM micro_events AS event
  WHERE event.status IN ('active','review_pending') AND event.event_started_at >= $5::timestamptz-interval '7 days'
  UNION
  SELECT event.id FROM ann_events AS ann JOIN micro_events AS event ON event.id=ann.micro_event_id
  WHERE event.status IN ('active','review_pending') AND event.event_started_at >= $5::timestamptz-interval '7 days'
)
SELECT event.id,event.version,
       similarity(event.primary_subject_key||' '||event.primary_action_key,$1||' '||$2)::float8 AS sparse_similarity,
       COALESCE(ann.dense_similarity,0)::float8 AS dense_similarity,
       (ann.micro_event_id IS NOT NULL)::boolean AS dense_available,
       CASE WHEN event.primary_subject_key=$1 THEN 1 ELSE 0 END::float8 AS entity_overlap,
       CASE WHEN event.primary_action_key=$2 THEN 1 ELSE 0 END::float8 AS action_overlap,
       CASE WHEN cardinality(event.location_keys)=0 OR cardinality($3::varchar[])=0 THEN .5
            WHEN event.location_keys && $3::varchar[] THEN 1 ELSE 0 END::float8 AS location_consistency,
       CASE WHEN cardinality(event.identifier_keys)=0 OR cardinality($4::varchar[])=0 THEN .5
            WHEN event.identifier_keys && $4::varchar[] THEN 1 ELSE 0 END::float8 AS identifier_consistency,
       exp(-abs(extract(epoch FROM (event.event_started_at-$5::timestamptz)))/259200)::float8 AS time_similarity,
       0::float8 AS lineage_relation,
       (similarity(event.primary_subject_key,$1)<.3
        OR (cardinality(event.location_keys)>0 AND cardinality($3::varchar[])>0 AND NOT event.location_keys && $3::varchar[])
        OR abs(extract(epoch FROM (event.event_started_at-$5::timestamptz)))>259200)::boolean AS hard_conflict,
       to_json(ARRAY_REMOVE(ARRAY[
         CASE WHEN similarity(event.primary_subject_key,$1)<.3 THEN 'entity_conflict' END,
         CASE WHEN cardinality(event.location_keys)>0 AND cardinality($3::varchar[])>0 AND NOT event.location_keys && $3::varchar[] THEN 'location_conflict' END,
         CASE WHEN abs(extract(epoch FROM (event.event_started_at-$5::timestamptz)))>259200 THEN 'time_conflict' END
       ],NULL)::varchar[]) AS hard_conflict_reasons
FROM candidate_events AS candidate
JOIN micro_events AS event ON event.id=candidate.id
LEFT JOIN ann_events AS ann ON ann.micro_event_id=event.id
ORDER BY similarity(event.primary_subject_key||' '||event.primary_action_key,$1||' '||$2)
         +COALESCE(ann.dense_similarity,0)
         +CASE WHEN event.primary_subject_key=$1 THEN 1 ELSE 0 END
         +CASE WHEN event.primary_action_key=$2 THEN 1 ELSE 0 END DESC,event.id
LIMIT 20`, target.PrimarySubjectKey, target.PrimaryActionKey, target.LocationKeys, target.IdentifierKeys,
		target.OccurredAt.UTC(), target.DocumentVersionID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	result := []eventapplication.MicroEventCandidateDTO{}
	for rows.Next() {
		var value eventapplication.MicroEventCandidateDTO
		if err := rows.Scan(&value.MicroEventID, &value.EventVersion, &value.Features.SparseSimilarity,
			&value.Features.DenseSimilarity, &value.DenseAvailable, &value.Features.EntityOverlap, &value.Features.ActionOverlap,
			&value.Features.LocationConsistency, &value.Features.IdentifierConsistency, &value.Features.TimeSimilarity,
			&value.Features.LineageRelation, &value.HardConflict,
			microEventStringArrayScan{destination: &value.HardConflictReasons}); err != nil {
			return nil, databaserepository.MapError(err)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return result, nil
}

func (repository *MicroEventRepository) CommitMicroEventMembership(ctx context.Context, command eventapplication.CommitMicroEventMembershipCommand) (eventapplication.CommitMicroEventMembershipResult, error) {
	if repository == nil || repository.runtime == nil || validateMicroEventCommit(command) != nil {
		return eventapplication.CommitMicroEventMembershipResult{}, eventapplication.ErrInvalidMicroEventContract
	}
	var result eventapplication.CommitMicroEventMembershipResult
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		replayed, found, replayErr := readMicroEventDecision(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if replayErr != nil {
			return replayErr
		}
		if found {
			if replayed.Decision.commandFingerprint != command.CommandFingerprint {
				return sharedrepository.ErrConflict
			}
			result, replayErr = microEventResultFromRecords(replayed.Event, replayed.Decision)
			return replayErr
		}
		eventRecord, eventErr := resolveMicroEvent(transactionCtx, transaction.SQL, command)
		if eventErr != nil {
			return eventErr
		}
		reasons, _ := json.Marshal(append([]string{}, command.ReasonCodes...))
		hardConflicts, _ := json.Marshal(append([]string{}, command.HardConflictReasons...))
		var candidate any
		if command.CandidateMicroEventID > 0 {
			candidate = command.CandidateMicroEventID
		}
		var decisionID int64
		err := transaction.SQL.QueryRowContext(transactionCtx, `
INSERT INTO micro_event_membership_decisions (
 content_family_id,document_match_decision_id,monitor_id,monitor_version_id,candidate_micro_event_id,
 resulting_micro_event_id,result_event_version,action,same_event_score,leading_margin,
 sparse_similarity,dense_similarity,entity_overlap,action_overlap,location_consistency,identifier_consistency,
 time_similarity,lineage_relation,hard_conflict_reasons,clustering_profile_version,reason_codes,
 decision_origin,idempotency_key,command_fingerprint
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19::jsonb,$20,$21::jsonb,'automatic',$22,$23)
RETURNING id`, command.ContentFamilyID, command.DocumentMatchDecisionID, command.MonitorID, command.MonitorVersionID,
			candidate, eventRecord.id, eventRecord.version, command.Action, command.SameEventScore, command.LeadingMargin,
			command.Features.SparseSimilarity, command.Features.DenseSimilarity, command.Features.EntityOverlap,
			command.Features.ActionOverlap, command.Features.LocationConsistency, command.Features.IdentifierConsistency,
			command.Features.TimeSimilarity, command.Features.LineageRelation, string(hardConflicts),
			command.ClusteringProfileVersion, string(reasons), command.IdempotencyKey, command.CommandFingerprint).Scan(&decisionID)
		if err != nil {
			return databaserepository.MapError(err)
		}
		if command.Action != "review" {
			if _, err := transaction.SQL.ExecContext(transactionCtx, `INSERT INTO micro_event_members
(micro_event_id,content_family_id,membership_decision_id,clustering_profile_version)
VALUES ($1,$2,$3,$4)`, eventRecord.id, command.ContentFamilyID, decisionID, command.ClusteringProfileVersion); err != nil {
				return databaserepository.MapError(err)
			}
		}
		stored, found, readErr := readMicroEventDecision(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if readErr != nil || !found {
			return readErr
		}
		result, readErr = microEventResultFromRecords(stored.Event, stored.Decision)
		return readErr
	})
	if err != nil {
		return eventapplication.CommitMicroEventMembershipResult{}, err
	}
	return result, nil
}

type microEventStoredDecision struct {
	Event    microEventRecord
	Decision microEventDecisionRecord
}

func readMicroEventDecision(ctx context.Context, executor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (microEventStoredDecision, bool, error) {
	var value microEventStoredDecision
	err := executor.QueryRowContext(ctx, `
SELECT event.id,decision.result_event_version,btrim(event.event_key),event.status,event.primary_subject_key,event.primary_action_key,
       to_json(event.location_keys),to_json(event.identifier_keys),event.event_started_at,event.clustering_profile_version,
       decision.id,decision.content_family_id,decision.document_match_decision_id,decision.resulting_micro_event_id,
       decision.result_event_version,decision.action,decision.same_event_score::float8,decision.leading_margin::float8,
       decision.sparse_similarity::float8,decision.dense_similarity::float8,decision.entity_overlap::float8,
       decision.action_overlap::float8,decision.location_consistency::float8,decision.identifier_consistency::float8,
       decision.time_similarity::float8,decision.lineage_relation::float8,decision.clustering_profile_version,
       decision.reason_codes,btrim(decision.command_fingerprint)
FROM micro_event_membership_decisions AS decision
JOIN micro_events AS event ON event.id=decision.resulting_micro_event_id
WHERE decision.idempotency_key=$1`, key).Scan(&value.Event.id, &value.Event.version, &value.Event.eventKey,
		&value.Event.status, &value.Event.subjectKey, &value.Event.actionKey,
		microEventStringArrayScan{destination: &value.Event.locationKeys},
		microEventStringArrayScan{destination: &value.Event.identifierKeys}, &value.Event.eventStartedAt, &value.Event.profileVersion,
		&value.Decision.id, &value.Decision.contentFamilyID, &value.Decision.documentMatchDecisionID,
		&value.Decision.microEventID, &value.Decision.eventVersion, &value.Decision.action,
		&value.Decision.sameEventScore, &value.Decision.leadingMargin, &value.Decision.features.SparseSimilarity,
		&value.Decision.features.DenseSimilarity, &value.Decision.features.EntityOverlap, &value.Decision.features.ActionOverlap,
		&value.Decision.features.LocationConsistency, &value.Decision.features.IdentifierConsistency,
		&value.Decision.features.TimeSimilarity, &value.Decision.features.LineageRelation, &value.Decision.profileVersion,
		&value.Decision.reasonCodesJSON, &value.Decision.commandFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return microEventStoredDecision{}, false, nil
	}
	if err != nil {
		return microEventStoredDecision{}, false, databaserepository.MapError(err)
	}
	return value, true, nil
}

func resolveMicroEvent(ctx context.Context, executor *sql.Tx, command eventapplication.CommitMicroEventMembershipCommand) (microEventRecord, error) {
	var record microEventRecord
	if command.Action == "create" {
		locationKeys := append([]string{}, command.LocationKeys...)
		identifierKeys := append([]string{}, command.IdentifierKeys...)
		err := executor.QueryRowContext(ctx, `INSERT INTO micro_events
(event_key,primary_subject_key,primary_action_key,location_keys,identifier_keys,event_started_at,clustering_profile_version)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id,version,btrim(event_key),status,primary_subject_key,primary_action_key,to_json(location_keys),to_json(identifier_keys),event_started_at,clustering_profile_version`,
			command.EventKey, command.PrimarySubjectKey, command.PrimaryActionKey, locationKeys,
			identifierKeys, command.OccurredAt.UTC(), command.ClusteringProfileVersion).Scan(&record.id, &record.version,
			&record.eventKey, &record.status, &record.subjectKey, &record.actionKey,
			microEventStringArrayScan{destination: &record.locationKeys},
			microEventStringArrayScan{destination: &record.identifierKeys}, &record.eventStartedAt, &record.profileVersion)
		if err != nil {
			return microEventRecord{}, databaserepository.MapError(err)
		}
		return record, nil
	}
	status := "active"
	if command.Action == "review" {
		status = "review_pending"
	}
	err := executor.QueryRowContext(ctx, `UPDATE micro_events SET version=version+1,status=$3,updated_at=now()
WHERE id=$1 AND version=$2 AND status IN ('active','review_pending')
RETURNING id,version,btrim(event_key),status,primary_subject_key,primary_action_key,to_json(location_keys),to_json(identifier_keys),event_started_at,clustering_profile_version`,
		command.CandidateMicroEventID, command.ExpectedEventVersion, status).Scan(&record.id, &record.version,
		&record.eventKey, &record.status, &record.subjectKey, &record.actionKey,
		microEventStringArrayScan{destination: &record.locationKeys},
		microEventStringArrayScan{destination: &record.identifierKeys}, &record.eventStartedAt, &record.profileVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return microEventRecord{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return microEventRecord{}, databaserepository.MapError(err)
	}
	return record, nil
}

func microEventResultFromRecords(event microEventRecord, decision microEventDecisionRecord) (eventapplication.CommitMicroEventMembershipResult, error) {
	decisionDTO, err := decision.dto()
	if err != nil {
		return eventapplication.CommitMicroEventMembershipResult{}, err
	}
	return eventapplication.CommitMicroEventMembershipResult{Event: event.dto(), Decision: decisionDTO}, nil
}

func validateMicroEventCommit(command eventapplication.CommitMicroEventMembershipCommand) error {
	if command.ContentFamilyID <= 0 || command.DocumentMatchDecisionID <= 0 || command.MonitorID <= 0 || command.MonitorVersionID <= 0 ||
		!validMicroEventHash(command.EventKey) || strings.TrimSpace(command.PrimarySubjectKey) == "" ||
		strings.TrimSpace(command.PrimaryActionKey) == "" || command.OccurredAt.IsZero() ||
		command.ClusteringProfileVersion != eventapplication.CanonicalMicroEventClusteringProfileVersion ||
		!validMicroEventAction(command.Action) || len(command.ReasonCodes) == 0 || strings.TrimSpace(command.IdempotencyKey) == "" ||
		!validMicroEventHash(command.CommandFingerprint) || !validMicroEventScore(command.SameEventScore) ||
		!validMicroEventScore(command.LeadingMargin) || !validMicroEventFeatures(command.Features) {
		return eventapplication.ErrInvalidMicroEventContract
	}
	if command.Action == "create" && (command.CandidateMicroEventID != 0 || command.ExpectedEventVersion != 0) {
		return eventapplication.ErrInvalidMicroEventContract
	}
	if command.Action != "create" && (command.CandidateMicroEventID <= 0 || command.ExpectedEventVersion <= 0) {
		return eventapplication.ErrInvalidMicroEventContract
	}
	return nil
}

func validMicroEventFeatures(value eventapplication.MicroEventFeaturesDTO) bool {
	return validMicroEventScore(value.SparseSimilarity) && validMicroEventScore(value.DenseSimilarity) &&
		validMicroEventScore(value.EntityOverlap) && validMicroEventScore(value.ActionOverlap) &&
		validMicroEventScore(value.LocationConsistency) && validMicroEventScore(value.IdentifierConsistency) &&
		validMicroEventScore(value.TimeSimilarity) && validMicroEventScore(value.LineageRelation)
}
func validMicroEventScore(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}
func validMicroEventAction(value string) bool {
	return value == "create" || value == "join" || value == "review"
}
func validMicroEventHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

type microEventQueryExecutor interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *MicroEventRepository) queryExecutor(ctx context.Context) microEventQueryExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}
func (repository *MicroEventRepository) queryRow(ctx context.Context, query string, arguments ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, arguments...)
	}
	return repository.runtime.SQL.QueryRowContext(ctx, query, arguments...)
}
func (repository *MicroEventRepository) withTransaction(ctx context.Context, operation func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return operation(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, operation)
}
