package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type StorylinePostgresRepository struct{ runtime *database.Runtime }

var _ eventapplication.StorylineRepository = (*StorylinePostgresRepository)(nil)

func NewStorylinePostgresRepository(runtime *database.Runtime) (*StorylinePostgresRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("storyline database runtime is required")
	}
	return &StorylinePostgresRepository{runtime: runtime}, nil
}

func (repository *StorylinePostgresRepository) ReadStorylineAssignmentTarget(ctx context.Context, query eventapplication.ReadStorylineAssignmentTargetQuery) (eventapplication.StorylineAssignmentTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.MicroEventID <= 0 || query.MicroEventVersion <= 0 ||
		query.RelationProfileVersion != eventapplication.CanonicalStorylineRelationProfileVersion {
		return eventapplication.StorylineAssignmentTargetDTO{}, eventapplication.ErrInvalidStorylineContract
	}
	var target eventapplication.StorylineAssignmentTargetDTO
	err := repository.queryRow(ctx, `SELECT id,version,status,primary_subject_key,primary_action_key,event_started_at
FROM micro_events WHERE id=$1 AND version=$2 AND status IN ('active','review_pending')`,
		query.MicroEventID, query.MicroEventVersion).Scan(&target.MicroEventID, &target.MicroEventVersion,
		&target.MicroEventStatus, &target.PrimarySubjectKey, &target.PrimaryActionKey, &target.EventStartedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return eventapplication.StorylineAssignmentTargetDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return eventapplication.StorylineAssignmentTargetDTO{}, databaserepository.MapError(err)
	}
	target.RelationProfileVersion = query.RelationProfileVersion
	idempotencyKey, _ := storylineAssignmentIdentity(query.MicroEventID, query.MicroEventVersion, query.RelationProfileVersion)
	if stored, found, readErr := readStorylineAssignment(ctx, repository.queryExecutor(ctx), idempotencyKey); readErr != nil {
		return eventapplication.StorylineAssignmentTargetDTO{}, readErr
	} else if found {
		value, conversionErr := storylineAssignmentResult(stored)
		if conversionErr != nil {
			return eventapplication.StorylineAssignmentTargetDTO{}, conversionErr
		}
		target.ExistingAssignment = &value
		return target, nil
	}
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, `
SELECT storyline.id,storyline.version,
       similarity(storyline.title,$1)::float8,
       CASE WHEN bool_or(event.primary_action_key=$2) THEN 1 ELSE 0 END::float8,
       exp(-abs(extract(epoch FROM ($3::timestamptz-max(event.event_started_at))))/2592000)::float8,
       max(event.event_started_at)
FROM storylines AS storyline
JOIN storyline_events AS relation ON relation.storyline_id=storyline.id AND relation.active
  AND relation.relation_profile_version=$4
JOIN micro_events AS event ON event.id=relation.micro_event_id AND event.status IN ('active','review_pending','closed')
WHERE storyline.status='active' AND relation.micro_event_id<>$5
  AND similarity(storyline.title,$1)>=.25
GROUP BY storyline.id,storyline.version,storyline.title
ORDER BY similarity(storyline.title,$1)+CASE WHEN bool_or(event.primary_action_key=$2) THEN 1 ELSE 0 END DESC,
         storyline.id
LIMIT 20`, target.PrimarySubjectKey, target.PrimaryActionKey, target.EventStartedAt.UTC(), query.RelationProfileVersion,
		target.MicroEventID)
	if err != nil {
		return eventapplication.StorylineAssignmentTargetDTO{}, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	target.Candidates = []eventapplication.StorylineCandidateDTO{}
	for rows.Next() {
		var candidate eventapplication.StorylineCandidateDTO
		if err := rows.Scan(&candidate.StorylineID, &candidate.StorylineVersion, &candidate.SubjectSimilarity,
			&candidate.ActionOverlap, &candidate.TimeRecency, &candidate.LatestEventAt); err != nil {
			return eventapplication.StorylineAssignmentTargetDTO{}, databaserepository.MapError(err)
		}
		target.Candidates = append(target.Candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return eventapplication.StorylineAssignmentTargetDTO{}, databaserepository.MapError(err)
	}
	return target, nil
}

func (repository *StorylinePostgresRepository) CommitStorylineAssignment(ctx context.Context, command eventapplication.CommitStorylineAssignmentCommand) (eventapplication.CommitStorylineAssignmentResult, error) {
	if repository == nil || repository.runtime == nil || validateStorylineCommit(command) != nil {
		return eventapplication.CommitStorylineAssignmentResult{}, eventapplication.ErrInvalidStorylineContract
	}
	var result eventapplication.CommitStorylineAssignmentResult
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		stored, found, err := readStorylineAssignment(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if stored.Relation.commandFingerprint != command.CommandFingerprint {
				return sharedrepository.ErrConflict
			}
			result, err = storylineAssignmentResult(stored)
			return err
		}
		storyline, err := resolveStoryline(transactionCtx, transaction.SQL, command)
		if err != nil {
			return err
		}
		reasons, _ := json.Marshal(append([]string{}, command.ReasonCodes...))
		var relationID int64
		if err := transaction.SQL.QueryRowContext(transactionCtx, `INSERT INTO storyline_events (
storyline_id,micro_event_id,source_micro_event_version,result_storyline_version,relation_type,relation_score,
relation_profile_version,reason_codes,decision_origin,storyline_key_snapshot,storyline_title_snapshot,
storyline_status_snapshot,idempotency_key,command_fingerprint)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,'automatic',$9,$10,$11,$12,$13) RETURNING id`, storyline.id,
			command.MicroEventID, command.MicroEventVersion, storyline.version, command.RelationType,
			command.RelationScore, command.RelationProfileVersion, string(reasons), storyline.storylineKey,
			storyline.title, storyline.status, command.IdempotencyKey, command.CommandFingerprint).Scan(&relationID); err != nil {
			return databaserepository.MapError(err)
		}
		stored, found, err = readStorylineAssignment(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if err != nil || !found {
			return err
		}
		result, err = storylineAssignmentResult(stored)
		return err
	})
	if err != nil {
		return eventapplication.CommitStorylineAssignmentResult{}, err
	}
	return result, nil
}

type storylineStoredAssignment struct {
	Storyline storylineRecord
	Relation  storylineEventRecord
}

type storylineRowReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readStorylineAssignment(ctx context.Context, executor storylineRowReader, idempotencyKey string) (storylineStoredAssignment, bool, error) {
	var value storylineStoredAssignment
	err := executor.QueryRowContext(ctx, `SELECT relation.storyline_id,relation.result_storyline_version,
       btrim(relation.storyline_key_snapshot),relation.storyline_title_snapshot,''::text,relation.storyline_status_snapshot,
       relation.relation_profile_version,relation.id,relation.storyline_id,relation.result_storyline_version,
       relation.micro_event_id,relation.source_micro_event_version,relation.relation_type,relation.relation_score::float8,
       relation.relation_profile_version,relation.reason_codes,relation.decision_origin,btrim(relation.command_fingerprint)
FROM storyline_events AS relation WHERE relation.idempotency_key=$1`, idempotencyKey).Scan(&value.Storyline.id,
		&value.Storyline.version, &value.Storyline.storylineKey, &value.Storyline.title, &value.Storyline.summary,
		&value.Storyline.status, &value.Storyline.relationProfileVersion, &value.Relation.id,
		&value.Relation.storylineID, &value.Relation.storylineVersion, &value.Relation.microEventID,
		&value.Relation.microEventVersion, &value.Relation.relationType, &value.Relation.relationScore,
		&value.Relation.relationProfileVersion, &value.Relation.reasonCodesJSON, &value.Relation.decisionOrigin,
		&value.Relation.commandFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return storylineStoredAssignment{}, false, nil
	}
	if err != nil {
		return storylineStoredAssignment{}, false, databaserepository.MapError(err)
	}
	return value, true, nil
}

func resolveStoryline(ctx context.Context, transaction *sql.Tx, command eventapplication.CommitStorylineAssignmentCommand) (storylineRecord, error) {
	var value storylineRecord
	if command.CreateNew {
		err := transaction.QueryRowContext(ctx, `INSERT INTO storylines (storyline_key,title,relation_profile_version)
VALUES ($1,$2,$3) RETURNING id,version,btrim(storyline_key),title,summary,status,relation_profile_version`,
			command.StorylineKey, command.Title, command.RelationProfileVersion).Scan(&value.id, &value.version,
			&value.storylineKey, &value.title, &value.summary, &value.status, &value.relationProfileVersion)
		if err != nil {
			return storylineRecord{}, databaserepository.MapError(err)
		}
		return value, nil
	}
	err := transaction.QueryRowContext(ctx, `UPDATE storylines SET version=version+1,updated_at=now()
WHERE id=$1 AND version=$2 AND status='active'
RETURNING id,version,btrim(storyline_key),title,summary,status,relation_profile_version`,
		command.CandidateStorylineID, command.ExpectedStorylineVersion).Scan(&value.id, &value.version,
		&value.storylineKey, &value.title, &value.summary, &value.status, &value.relationProfileVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return storylineRecord{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return storylineRecord{}, databaserepository.MapError(err)
	}
	return value, nil
}

func storylineAssignmentResult(value storylineStoredAssignment) (eventapplication.CommitStorylineAssignmentResult, error) {
	relation, err := value.Relation.dto()
	if err != nil {
		return eventapplication.CommitStorylineAssignmentResult{}, err
	}
	return eventapplication.CommitStorylineAssignmentResult{Storyline: value.Storyline.dto(), Relation: relation}, nil
}

func validateStorylineCommit(command eventapplication.CommitStorylineAssignmentCommand) error {
	if command.MicroEventID <= 0 || command.MicroEventVersion <= 0 || !validStorylineHash(command.StorylineKey) ||
		strings.TrimSpace(command.Title) == "" || len(command.Title) > 300 ||
		command.RelationProfileVersion != eventapplication.CanonicalStorylineRelationProfileVersion ||
		!validStorylineRelation(command.RelationType) || !validStorylineScore(command.RelationScore) ||
		len(command.ReasonCodes) == 0 || strings.TrimSpace(command.IdempotencyKey) == "" ||
		!validStorylineHash(command.CommandFingerprint) {
		return eventapplication.ErrInvalidStorylineContract
	}
	if command.CreateNew && (command.CandidateStorylineID != 0 || command.ExpectedStorylineVersion != 0) ||
		!command.CreateNew && (command.CandidateStorylineID <= 0 || command.ExpectedStorylineVersion <= 0) {
		return eventapplication.ErrInvalidStorylineContract
	}
	return nil
}

func storylineAssignmentIdentity(eventID, eventVersion int64, profile string) (string, string) {
	dummy := eventapplication.CommitStorylineAssignmentCommand{MicroEventID: eventID, MicroEventVersion: eventVersion,
		RelationProfileVersion: profile}
	key, fingerprint := storylineMutationIdentityForRepository(dummy)
	return key, fingerprint
}

func storylineMutationIdentityForRepository(command eventapplication.CommitStorylineAssignmentCommand) (string, string) {
	// This mirrors the public use-case key only. The fingerprint returned here is
	// intentionally unused while resolving an existing immutable receipt.
	keyDigest := fmt.Sprintf("%x", eventIdentityDigest(command.MicroEventID, command.MicroEventVersion, command.RelationProfileVersion))
	return "storyline-event-" + keyDigest[:32], ""
}

func eventIdentityDigest(eventID, eventVersion int64, profile string) [32]byte {
	return sha256Sum(fmt.Sprintf("storyline-event:%d:%d:%s", eventID, eventVersion, profile))
}

func sha256Sum(value string) [32]byte {
	// kept private to avoid sharing persistence identities outside the use case
	return sha256.Sum256([]byte(value))
}

func validStorylineRelation(value string) bool {
	return value == "continues" || value == "causes" || value == "responds_to" || value == "updates" || value == "related"
}
func validStorylineScore(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}
func validStorylineHash(value string) bool {
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

type storylineQueryExecutor interface {
	storylineRowReader
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (repository *StorylinePostgresRepository) queryExecutor(ctx context.Context) storylineQueryExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func (repository *StorylinePostgresRepository) queryRow(ctx context.Context, query string, arguments ...any) *sql.Row {
	return repository.queryExecutor(ctx).QueryRowContext(ctx, query, arguments...)
}

func (repository *StorylinePostgresRepository) withTransaction(ctx context.Context, operation func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return operation(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, operation)
}
