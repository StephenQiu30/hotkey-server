package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type MicroEventGovernancePostgresRepository struct{ runtime *database.Runtime }

var _ eventapplication.MicroEventGovernanceRepository = (*MicroEventGovernancePostgresRepository)(nil)

func NewMicroEventGovernancePostgresRepository(runtime *database.Runtime) (*MicroEventGovernancePostgresRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("micro-event governance database runtime is required")
	}
	return &MicroEventGovernancePostgresRepository{runtime: runtime}, nil
}

func (repository *MicroEventGovernancePostgresRepository) ApplyMicroEventGovernance(ctx context.Context, command eventapplication.ApplyMicroEventGovernanceCommand) (eventapplication.ApplyMicroEventGovernanceResult, error) {
	if repository == nil || repository.runtime == nil || len(command.CommandFingerprint) != 64 {
		return eventapplication.ApplyMicroEventGovernanceResult{}, eventapplication.ErrInvalidMicroEventGovernanceContract
	}
	var result eventapplication.ApplyMicroEventGovernanceResult
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		stored, found, err := readMicroEventGovernanceResult(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if stored.commandFingerprint != command.CommandFingerprint {
				return sharedrepository.ErrConflict
			}
			result = stored.result
			return nil
		}
		if err := authorizeMicroEventGovernor(transactionCtx, transaction.SQL, command.ActorUserID); err != nil {
			return err
		}
		source, err := lockGovernedMicroEvent(transactionCtx, transaction.SQL, command.MicroEventID, command.ExpectedEventVersion)
		if err != nil {
			return err
		}
		var target *microEventRecord
		if command.TargetMicroEventID > 0 {
			locked, lockErr := lockGovernedMicroEvent(transactionCtx, transaction.SQL, command.TargetMicroEventID,
				command.ExpectedTargetEventVersion)
			if lockErr != nil {
				return lockErr
			}
			target = &locked
		}
		var resultDecisionID, resultMemberVersion int64
		switch command.Action {
		case "close_event":
			source, err = transitionGovernedMicroEvent(transactionCtx, transaction.SQL, source, "closed", 0)
		case "reopen_event":
			source, err = transitionGovernedMicroEvent(transactionCtx, transaction.SQL, source, "active", 0)
		case "same_event", "different_event":
			var original microEventGovernanceDecisionRecord
			original, err = lockGovernanceDecision(transactionCtx, transaction.SQL, command.MembershipDecisionID,
				command.MicroEventID, command.ContentFamilyID, true)
			if err == nil {
				if command.Action == "same_event" {
					source, err = transitionGovernedMicroEvent(transactionCtx, transaction.SQL, source, "active", 0)
					if err == nil {
						resultDecisionID, resultMemberVersion, err = insertManualMicroEventMember(transactionCtx,
							transaction.SQL, original, source, command, false, 1)
					}
				} else {
					source, err = transitionGovernedMicroEvent(transactionCtx, transaction.SQL, source, "active", 0)
					if err == nil {
						var created microEventRecord
						created, err = createGovernedMicroEvent(transactionCtx, transaction.SQL, source, command)
						if err == nil {
							target = &created
							resultDecisionID, resultMemberVersion, err = insertManualMicroEventMember(transactionCtx,
								transaction.SQL, original, created, command, true, 1)
						}
					}
				}
			}
		case "move_member", "split_event", "withdraw":
			var member microEventGovernanceMemberRecord
			member, err = lockGovernedMicroEventMember(transactionCtx, transaction.SQL, command)
			if err == nil {
				source, err = retireGovernedMemberAndAdvanceEvent(transactionCtx, transaction.SQL, source, member)
			}
			if err == nil && command.Action == "move_member" {
				*target, err = advanceGovernedTargetEvent(transactionCtx, transaction.SQL, *target)
				if err == nil {
					resultDecisionID, resultMemberVersion, err = insertManualMicroEventMember(transactionCtx,
						transaction.SQL, member.decision, *target, command, false, member.version+1)
				}
			}
			if err == nil && command.Action == "split_event" {
				var created microEventRecord
				created, err = createGovernedMicroEvent(transactionCtx, transaction.SQL, source, command)
				if err == nil {
					target = &created
					resultDecisionID, resultMemberVersion, err = insertManualMicroEventMember(transactionCtx,
						transaction.SQL, member.decision, created, command, true, member.version+1)
				}
			}
		case "merge_events":
			if source.status == "merged" || target == nil || target.status == "merged" {
				err = sharedrepository.ErrConflict
				break
			}
			*target, err = advanceGovernedTargetEvent(transactionCtx, transaction.SQL, *target)
			if err == nil {
				err = mergeGovernedMicroEventMembers(transactionCtx, transaction.SQL, source, *target, command)
			}
			if err == nil {
				source, err = transitionGovernedMicroEvent(transactionCtx, transaction.SQL, source, "merged", target.id)
			}
		default:
			err = eventapplication.ErrInvalidMicroEventGovernanceContract
		}
		if err != nil {
			return err
		}
		feedbackID, err := insertMicroEventGovernanceFeedback(transactionCtx, transaction.SQL, command, source, target,
			resultDecisionID, resultMemberVersion)
		if err != nil {
			return err
		}
		stored, found, err = readMicroEventGovernanceResult(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if err != nil || !found || stored.result.Feedback.ID != feedbackID {
			return err
		}
		result = stored.result
		return nil
	})
	if err != nil {
		return eventapplication.ApplyMicroEventGovernanceResult{}, err
	}
	return result, nil
}

type microEventGovernanceDecisionRecord struct {
	id, contentFamilyID, documentMatchDecisionID, monitorID, monitorVersionID int64
	clusteringProfileVersion                                                  string
}

type microEventGovernanceMemberRecord struct {
	id, version int64
	decision    microEventGovernanceDecisionRecord
}

func authorizeMicroEventGovernor(ctx context.Context, transaction *sql.Tx, actorID int64) error {
	var allowed bool
	if err := transaction.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users
WHERE id=$1 AND deleted_at IS NULL AND status='active' AND role IN ('editor','admin'))`, actorID).Scan(&allowed); err != nil {
		return databaserepository.MapError(err)
	}
	if !allowed {
		return eventapplication.ErrMicroEventGovernanceForbidden
	}
	return nil
}

func lockGovernedMicroEvent(ctx context.Context, transaction *sql.Tx, id, version int64) (microEventRecord, error) {
	var value microEventRecord
	err := transaction.QueryRowContext(ctx, `SELECT id,version,btrim(event_key),status,primary_subject_key,
primary_action_key,to_json(location_keys),to_json(identifier_keys),event_started_at,clustering_profile_version
FROM micro_events WHERE id=$1 AND version=$2 FOR UPDATE`, id, version).Scan(&value.id, &value.version,
		&value.eventKey, &value.status, &value.subjectKey, &value.actionKey,
		microEventStringArrayScan{destination: &value.locationKeys}, microEventStringArrayScan{destination: &value.identifierKeys},
		&value.eventStartedAt, &value.profileVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return microEventRecord{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return microEventRecord{}, databaserepository.MapError(err)
	}
	return value, nil
}

func lockGovernanceDecision(ctx context.Context, transaction *sql.Tx, decisionID, eventID, familyID int64, requireReview bool) (microEventGovernanceDecisionRecord, error) {
	var value microEventGovernanceDecisionRecord
	var action string
	err := transaction.QueryRowContext(ctx, `SELECT id,content_family_id,document_match_decision_id,monitor_id,
monitor_version_id,clustering_profile_version,action FROM micro_event_membership_decisions
WHERE id=$1 AND resulting_micro_event_id=$2 AND content_family_id=$3 FOR KEY SHARE`, decisionID, eventID, familyID).Scan(
		&value.id, &value.contentFamilyID, &value.documentMatchDecisionID, &value.monitorID,
		&value.monitorVersionID, &value.clusteringProfileVersion, &action)
	if errors.Is(err, sql.ErrNoRows) {
		return microEventGovernanceDecisionRecord{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return microEventGovernanceDecisionRecord{}, databaserepository.MapError(err)
	}
	if requireReview && action != "review" {
		return microEventGovernanceDecisionRecord{}, sharedrepository.ErrConflict
	}
	return value, nil
}

func lockGovernedMicroEventMember(ctx context.Context, transaction *sql.Tx, command eventapplication.ApplyMicroEventGovernanceCommand) (microEventGovernanceMemberRecord, error) {
	var value microEventGovernanceMemberRecord
	err := transaction.QueryRowContext(ctx, `SELECT member.id,member.version,decision.id,decision.content_family_id,
decision.document_match_decision_id,decision.monitor_id,decision.monitor_version_id,decision.clustering_profile_version
FROM micro_event_members AS member
JOIN micro_event_membership_decisions AS decision ON decision.id=member.membership_decision_id
WHERE member.micro_event_id=$1 AND member.content_family_id=$2 AND member.membership_decision_id=$3
  AND member.version=$4 AND member.active FOR UPDATE`, command.MicroEventID, command.ContentFamilyID,
		command.MembershipDecisionID, command.ExpectedMemberVersion).Scan(&value.id, &value.version,
		&value.decision.id, &value.decision.contentFamilyID, &value.decision.documentMatchDecisionID,
		&value.decision.monitorID, &value.decision.monitorVersionID, &value.decision.clusteringProfileVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return microEventGovernanceMemberRecord{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return microEventGovernanceMemberRecord{}, databaserepository.MapError(err)
	}
	return value, nil
}

func transitionGovernedMicroEvent(ctx context.Context, transaction *sql.Tx, event microEventRecord, status string, mergedIntoID int64) (microEventRecord, error) {
	legal := status == "closed" && (event.status == "active" || event.status == "review_pending") ||
		status == "active" && (event.status == "closed" || event.status == "review_pending") ||
		status == "merged" && event.status != "merged"
	if !legal {
		return microEventRecord{}, sharedrepository.ErrConflict
	}
	var mergedInto any
	if mergedIntoID > 0 {
		mergedInto = mergedIntoID
	}
	endedAt := any(nil)
	if status == "closed" || status == "merged" {
		endedAt = time.Now().UTC()
	}
	return updateGovernedMicroEvent(ctx, transaction, event, status, mergedInto, endedAt)
}

func advanceGovernedTargetEvent(ctx context.Context, transaction *sql.Tx, event microEventRecord) (microEventRecord, error) {
	if event.status != "active" && event.status != "review_pending" {
		return microEventRecord{}, sharedrepository.ErrConflict
	}
	return updateGovernedMicroEvent(ctx, transaction, event, "active", nil, nil)
}

func updateGovernedMicroEvent(ctx context.Context, transaction *sql.Tx, event microEventRecord, status string, mergedInto, endedAt any) (microEventRecord, error) {
	var value microEventRecord
	err := transaction.QueryRowContext(ctx, `UPDATE micro_events
SET version=version+1,status=$3,merged_into_micro_event_id=$4,event_ended_at=$5,updated_at=now()
WHERE id=$1 AND version=$2
RETURNING id,version,btrim(event_key),status,primary_subject_key,primary_action_key,to_json(location_keys),
to_json(identifier_keys),event_started_at,clustering_profile_version`, event.id, event.version, status, mergedInto, endedAt).Scan(
		&value.id, &value.version, &value.eventKey, &value.status, &value.subjectKey, &value.actionKey,
		microEventStringArrayScan{destination: &value.locationKeys}, microEventStringArrayScan{destination: &value.identifierKeys},
		&value.eventStartedAt, &value.profileVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return microEventRecord{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return microEventRecord{}, databaserepository.MapError(err)
	}
	return value, nil
}

func retireGovernedMemberAndAdvanceEvent(ctx context.Context, transaction *sql.Tx, event microEventRecord, member microEventGovernanceMemberRecord) (microEventRecord, error) {
	if _, err := transaction.ExecContext(ctx, `UPDATE micro_event_members
SET version=version+1,active=false,retired_at=now() WHERE id=$1 AND version=$2 AND active`, member.id, member.version); err != nil {
		return microEventRecord{}, databaserepository.MapError(err)
	}
	var remaining int
	if err := transaction.QueryRowContext(ctx, `SELECT count(*) FROM micro_event_members
WHERE micro_event_id=$1 AND active`, event.id).Scan(&remaining); err != nil {
		return microEventRecord{}, databaserepository.MapError(err)
	}
	if remaining == 0 {
		return updateGovernedMicroEvent(ctx, transaction, event, "closed", nil, time.Now().UTC())
	}
	return updateGovernedMicroEvent(ctx, transaction, event, event.status, nil, nil)
}

func createGovernedMicroEvent(ctx context.Context, transaction *sql.Tx, source microEventRecord, command eventapplication.ApplyMicroEventGovernanceCommand) (microEventRecord, error) {
	digest := sha256.Sum256([]byte(fmt.Sprintf("micro-event-governance:%s:%d", command.IdempotencyKey, command.ContentFamilyID)))
	var value microEventRecord
	err := transaction.QueryRowContext(ctx, `INSERT INTO micro_events
(event_key,primary_subject_key,primary_action_key,location_keys,identifier_keys,event_started_at,clustering_profile_version)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id,version,btrim(event_key),status,primary_subject_key,primary_action_key,to_json(location_keys),
to_json(identifier_keys),event_started_at,clustering_profile_version`, hex.EncodeToString(digest[:]), source.subjectKey,
		source.actionKey, source.locationKeys, source.identifierKeys, source.eventStartedAt, source.profileVersion).Scan(
		&value.id, &value.version, &value.eventKey, &value.status, &value.subjectKey, &value.actionKey,
		microEventStringArrayScan{destination: &value.locationKeys}, microEventStringArrayScan{destination: &value.identifierKeys},
		&value.eventStartedAt, &value.profileVersion)
	if err != nil {
		return microEventRecord{}, databaserepository.MapError(err)
	}
	return value, nil
}

func insertManualMicroEventMember(ctx context.Context, transaction *sql.Tx, original microEventGovernanceDecisionRecord,
	target microEventRecord, command eventapplication.ApplyMicroEventGovernanceCommand, create bool, memberVersion int64) (int64, int64, error) {
	action := "join"
	var candidate any = target.id
	if create {
		action = "create"
		candidate = nil
	}
	reasons, _ := json.Marshal([]string{"manual_" + command.Action, command.ReasonCode})
	decisionKeyDigest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", command.IdempotencyKey, original.contentFamilyID)))
	fingerprintDigest := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s", command.CommandFingerprint,
		original.id, target.id, action)))
	var decisionID int64
	err := transaction.QueryRowContext(ctx, `INSERT INTO micro_event_membership_decisions (
content_family_id,document_match_decision_id,monitor_id,monitor_version_id,candidate_micro_event_id,
resulting_micro_event_id,result_event_version,action,same_event_score,leading_margin,sparse_similarity,dense_similarity,
entity_overlap,action_overlap,location_consistency,identifier_consistency,time_similarity,lineage_relation,
hard_conflict_reasons,clustering_profile_version,reason_codes,decision_origin,actor_user_id,idempotency_key,command_fingerprint)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,1,1,0,1,1,1,.5,1,0,'[]'::jsonb,$10,$11::jsonb,'manual',$12,$13,$14)
RETURNING id`, original.contentFamilyID, original.documentMatchDecisionID, original.monitorID, original.monitorVersionID,
		candidate, target.id, target.version, action, map[bool]float64{true: 0, false: 1}[create],
		original.clusteringProfileVersion, string(reasons), command.ActorUserID,
		"micro-governance-member-"+hex.EncodeToString(decisionKeyDigest[:16]), hex.EncodeToString(fingerprintDigest[:])).Scan(&decisionID)
	if err != nil {
		return 0, 0, databaserepository.MapError(err)
	}
	var resultVersion int64
	err = transaction.QueryRowContext(ctx, `INSERT INTO micro_event_members
(version,micro_event_id,content_family_id,membership_decision_id,clustering_profile_version)
VALUES ($1,$2,$3,$4,$5) RETURNING version`, memberVersion, target.id, original.contentFamilyID, decisionID,
		original.clusteringProfileVersion).Scan(&resultVersion)
	if err != nil {
		return 0, 0, databaserepository.MapError(err)
	}
	return decisionID, resultVersion, nil
}

func mergeGovernedMicroEventMembers(ctx context.Context, transaction *sql.Tx, source, target microEventRecord, command eventapplication.ApplyMicroEventGovernanceCommand) error {
	rows, err := transaction.QueryContext(ctx, `SELECT member.id,member.version,decision.id,decision.content_family_id,
decision.document_match_decision_id,decision.monitor_id,decision.monitor_version_id,decision.clustering_profile_version
FROM micro_event_members AS member JOIN micro_event_membership_decisions AS decision ON decision.id=member.membership_decision_id
WHERE member.micro_event_id=$1 AND member.active ORDER BY member.id FOR UPDATE`, source.id)
	if err != nil {
		return databaserepository.MapError(err)
	}
	members := []microEventGovernanceMemberRecord{}
	for rows.Next() {
		var member microEventGovernanceMemberRecord
		if err := rows.Scan(&member.id, &member.version, &member.decision.id, &member.decision.contentFamilyID,
			&member.decision.documentMatchDecisionID, &member.decision.monitorID, &member.decision.monitorVersionID,
			&member.decision.clusteringProfileVersion); err != nil {
			rows.Close()
			return databaserepository.MapError(err)
		}
		members = append(members, member)
	}
	if err := rows.Close(); err != nil {
		return databaserepository.MapError(err)
	}
	if len(members) == 0 {
		return sharedrepository.ErrConflict
	}
	for _, member := range members {
		if _, err := transaction.ExecContext(ctx, `UPDATE micro_event_members SET version=version+1,active=false,retired_at=now()
WHERE id=$1 AND version=$2 AND active`, member.id, member.version); err != nil {
			return databaserepository.MapError(err)
		}
		if _, _, err := insertManualMicroEventMember(ctx, transaction, member.decision, target, command, false, member.version+1); err != nil {
			return err
		}
	}
	return nil
}

func insertMicroEventGovernanceFeedback(ctx context.Context, transaction *sql.Tx,
	command eventapplication.ApplyMicroEventGovernanceCommand, source microEventRecord, target *microEventRecord,
	resultDecisionID, resultMemberVersion int64) (int64, error) {
	var membershipDecision, family, inputTarget, inputTargetVersion, resultTargetID, resultTargetVersion, resultTargetStatus any
	if command.MembershipDecisionID > 0 {
		membershipDecision = command.MembershipDecisionID
		family = command.ContentFamilyID
	}
	if command.TargetMicroEventID > 0 {
		inputTarget = command.TargetMicroEventID
		inputTargetVersion = command.ExpectedTargetEventVersion
	}
	if target != nil {
		resultTargetID = target.id
		resultTargetVersion = target.version
		resultTargetStatus = target.status
	}
	var resultDecision, resultMember any
	if resultDecisionID > 0 {
		resultDecision = resultDecisionID
		resultMember = resultMemberVersion
	}
	var feedbackID int64
	err := transaction.QueryRowContext(ctx, `INSERT INTO micro_event_feedbacks (
membership_decision_id,micro_event_id,original_event_version,content_family_id,actor_user_id,feedback_type,
target_micro_event_id,target_event_version,result_micro_event_id,result_event_version,result_event_status,
result_target_micro_event_id,result_target_event_version,result_target_event_status,result_membership_decision_id,
result_member_version,governance_profile_version,reason_code,note,idempotency_key,command_fingerprint)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
RETURNING id`, membershipDecision, command.MicroEventID, command.ExpectedEventVersion, family, command.ActorUserID,
		command.Action, inputTarget, inputTargetVersion, source.id, source.version, source.status, resultTargetID,
		resultTargetVersion, resultTargetStatus, resultDecision, resultMember, command.GovernanceProfileVersion,
		command.ReasonCode, command.Note, command.IdempotencyKey, command.CommandFingerprint).Scan(&feedbackID)
	if err != nil {
		return 0, databaserepository.MapError(err)
	}
	return feedbackID, nil
}

type storedMicroEventGovernanceResult struct {
	result             eventapplication.ApplyMicroEventGovernanceResult
	commandFingerprint string
}

func readMicroEventGovernanceResult(ctx context.Context, transaction *sql.Tx, idempotencyKey string) (storedMicroEventGovernanceResult, bool, error) {
	var stored storedMicroEventGovernanceResult
	var membershipDecision, family, targetID, targetVersion, resultTargetID, resultTargetVersion,
		resultDecision, resultMember sql.NullInt64
	var resultTargetStatus sql.NullString
	err := transaction.QueryRowContext(ctx, `SELECT feedback.id,feedback.feedback_type,feedback.actor_user_id,
feedback.micro_event_id,feedback.original_event_version,feedback.membership_decision_id,feedback.content_family_id,
feedback.target_micro_event_id,feedback.target_event_version,feedback.result_micro_event_id,feedback.result_event_version,
feedback.result_target_micro_event_id,feedback.result_target_event_version,feedback.result_target_event_status,
feedback.result_membership_decision_id,
feedback.result_member_version,feedback.governance_profile_version,feedback.reason_code,feedback.note,
feedback.idempotency_key,btrim(feedback.command_fingerprint),source.id,feedback.result_event_version,
btrim(source.event_key),feedback.result_event_status,source.primary_subject_key,source.primary_action_key,
to_json(source.location_keys),to_json(source.identifier_keys),source.event_started_at,source.clustering_profile_version
FROM micro_event_feedbacks AS feedback
JOIN micro_events AS source ON source.id=feedback.result_micro_event_id
WHERE feedback.idempotency_key=$1`, idempotencyKey).Scan(&stored.result.Feedback.ID, &stored.result.Feedback.Action,
		&stored.result.Feedback.ActorUserID, &stored.result.Feedback.MicroEventID,
		&stored.result.Feedback.OriginalEventVersion, &membershipDecision, &family, &targetID, &targetVersion,
		&stored.result.Feedback.ResultMicroEventID, &stored.result.Feedback.ResultEventVersion, &resultTargetID,
		&resultTargetVersion, &resultTargetStatus, &resultDecision, &resultMember, &stored.result.Feedback.GovernanceProfileVersion,
		&stored.result.Feedback.ReasonCode, &stored.result.Feedback.Note, &stored.result.Feedback.IdempotencyKey,
		&stored.commandFingerprint, &stored.result.SourceEvent.ID, &stored.result.SourceEvent.Version,
		&stored.result.SourceEvent.EventKey, &stored.result.SourceEvent.Status, &stored.result.SourceEvent.PrimarySubjectKey,
		&stored.result.SourceEvent.PrimaryActionKey, microEventStringArrayScan{destination: &stored.result.SourceEvent.LocationKeys},
		microEventStringArrayScan{destination: &stored.result.SourceEvent.IdentifierKeys}, &stored.result.SourceEvent.EventStartedAt,
		&stored.result.SourceEvent.ClusteringProfileVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return storedMicroEventGovernanceResult{}, false, nil
	}
	if err != nil {
		return storedMicroEventGovernanceResult{}, false, databaserepository.MapError(err)
	}
	stored.result.Feedback.MembershipDecisionID = membershipDecision.Int64
	stored.result.Feedback.ContentFamilyID = family.Int64
	stored.result.Feedback.TargetMicroEventID = targetID.Int64
	stored.result.Feedback.TargetEventVersion = targetVersion.Int64
	stored.result.Feedback.ResultTargetMicroEventID = resultTargetID.Int64
	stored.result.Feedback.ResultTargetEventVersion = resultTargetVersion.Int64
	stored.result.Feedback.ResultMembershipDecisionID = resultDecision.Int64
	stored.result.Feedback.ResultMemberVersion = resultMember.Int64
	if resultTargetID.Valid {
		var target microEventRecord
		err := transaction.QueryRowContext(ctx, `SELECT id,$2::bigint,btrim(event_key),$3::varchar,primary_subject_key,primary_action_key,
to_json(location_keys),to_json(identifier_keys),event_started_at,clustering_profile_version FROM micro_events WHERE id=$1`,
			resultTargetID.Int64, resultTargetVersion.Int64, resultTargetStatus.String).Scan(&target.id, &target.version,
			&target.eventKey, &target.status, &target.subjectKey, &target.actionKey,
			microEventStringArrayScan{destination: &target.locationKeys}, microEventStringArrayScan{destination: &target.identifierKeys},
			&target.eventStartedAt, &target.profileVersion)
		if err != nil {
			return storedMicroEventGovernanceResult{}, false, databaserepository.MapError(err)
		}
		dto := target.dto()
		stored.result.TargetEvent = &dto
	}
	return stored, true, nil
}
