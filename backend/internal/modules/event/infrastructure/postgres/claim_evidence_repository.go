package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type ClaimEvidencePostgresRepository struct{ runtime *database.Runtime }

var _ eventapplication.ClaimEvidenceRepository = (*ClaimEvidencePostgresRepository)(nil)

func NewClaimEvidencePostgresRepository(runtime *database.Runtime) (*ClaimEvidencePostgresRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("claim evidence database runtime is required")
	}
	return &ClaimEvidencePostgresRepository{runtime: runtime}, nil
}

func (repository *ClaimEvidencePostgresRepository) ReadAutomaticClaimEvidenceTarget(ctx context.Context, query eventapplication.AutomaticClaimEvidenceTargetQuery) (eventapplication.AutomaticClaimEvidenceTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.MicroEventID <= 0 || query.ExpectedEventVersion <= 0 ||
		query.DocumentVersionID <= 0 || query.DecisionAt.IsZero() {
		return eventapplication.AutomaticClaimEvidenceTargetDTO{}, eventapplication.ErrInvalidAutomaticClaimEvidenceContract
	}
	var value eventapplication.AutomaticClaimEvidenceTargetDTO
	err := repository.queryRow(ctx, `
SELECT event.id,event.version,event.event_key,document.id,version.id,artifact.artifact_type,
       btrim(artifact.transformer_profile_sha256),artifact.mime_type,btrim(artifact.sha256),artifact.size_bytes,
       artifact.retention_until,true,$4::timestamptz
FROM micro_events AS event
JOIN document_versions AS version ON version.id=$3 AND version.lifecycle_state='readable'
JOIN documents AS document ON document.id=version.document_id
JOIN derived_artifacts AS artifact ON artifact.document_version_id=version.id
  AND artifact.source_connection_id=document.source_connection_id
  AND artifact.artifact_type='plaintext' AND artifact.lifecycle_state='derived_available' AND artifact.active
  AND artifact.sha256=version.content_sha256 AND artifact.retention_until>$4
  AND current_rights_action_allowed(artifact.store_derived_rights_decision_id,document.source_connection_id,
      'document_version',version.id::text,version.content_sha256,'store_derived',$4)
  AND current_rights_action_allowed(artifact.retain_rights_decision_id,document.source_connection_id,
      'document_version',version.id::text,version.content_sha256,'retain',$4)
WHERE event.id=$1 AND event.version=$2
  AND EXISTS (
    SELECT 1 FROM micro_event_members AS event_member
    JOIN content_family_members AS family_member ON family_member.family_id=event_member.content_family_id
    WHERE event_member.micro_event_id=event.id AND event_member.active
      AND family_member.document_version_id=version.id AND family_member.active
  )
  AND EXISTS (
    SELECT 1 FROM source_rights_decisions AS decision
    WHERE decision.source_connection_id=document.source_connection_id
      AND decision.subject_type='document_version' AND decision.subject_key=version.id::text
      AND decision.input_digest=version.content_sha256 AND decision.action='send_external_model'
      AND current_rights_action_allowed(decision.id,document.source_connection_id,'document_version',
          version.id::text,version.content_sha256,'send_external_model',$4)
  )`, query.MicroEventID, query.ExpectedEventVersion, query.DocumentVersionID, query.DecisionAt.UTC()).Scan(
		&value.MicroEventID, &value.EventVersion, &value.EventKey, &value.Artifact.DocumentID,
		&value.Artifact.DocumentVersionID, &value.Artifact.ArtifactType, &value.Artifact.TransformerProfileSHA256,
		&value.Artifact.MIMEType, &value.Artifact.PlaintextSHA256, &value.Artifact.SizeBytes,
		&value.Artifact.RetentionUntil, &value.ExternalModelAllowed, &value.DecisionAt)
	if errors.Is(err, sql.ErrNoRows) {
		return eventapplication.AutomaticClaimEvidenceTargetDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return eventapplication.AutomaticClaimEvidenceTargetDTO{}, databaserepository.MapError(err)
	}
	value.Artifact.RetentionUntil, value.DecisionAt = value.Artifact.RetentionUntil.UTC(), value.DecisionAt.UTC()
	return value, nil
}

type claimEvidenceFeedbackRecord struct {
	id, version, claimID, originalEvidenceID, resultEvidenceID    int64
	targetDocumentVersionID, originalSelectorID, resultSelectorID int64
	originalRelation, resultRelation                              string
	actorUserID, expectedClaimVersion                             int64
	reasonCode, note, commandFingerprint                          string
	createdAt                                                     time.Time
}

func (record claimEvidenceFeedbackRecord) dto() eventapplication.ClaimEvidenceFeedbackDTO {
	return eventapplication.ClaimEvidenceFeedbackDTO{ID: record.id, Version: record.version, ClaimID: record.claimID,
		OriginalClaimEvidenceVersionID: record.originalEvidenceID, ResultClaimEvidenceVersionID: record.resultEvidenceID,
		TargetDocumentVersionID: record.targetDocumentVersionID, OriginalTextQuoteSelectorID: record.originalSelectorID,
		ResultTextQuoteSelectorID: record.resultSelectorID, OriginalRelation: record.originalRelation,
		ResultRelation: record.resultRelation, ActorUserID: record.actorUserID, ExpectedClaimVersion: record.expectedClaimVersion,
		ReasonCode: record.reasonCode, Note: record.note, CreatedAt: record.createdAt.UTC()}
}

func readClaimEvidenceFeedback(ctx context.Context, tx *sql.Tx, key string) (claimEvidenceFeedbackRecord, bool, error) {
	var record claimEvidenceFeedbackRecord
	err := tx.QueryRowContext(ctx, `SELECT id,version,claim_id,original_claim_evidence_version_id,result_claim_evidence_version_id,
target_document_version_id,original_text_quote_selector_id,result_text_quote_selector_id,original_relation,result_relation,
actor_user_id,expected_claim_version,reason_code,note,command_fingerprint,created_at
FROM claim_evidence_feedbacks WHERE idempotency_key=$1 FOR KEY SHARE`, key).Scan(&record.id, &record.version, &record.claimID,
		&record.originalEvidenceID, &record.resultEvidenceID, &record.targetDocumentVersionID, &record.originalSelectorID,
		&record.resultSelectorID, &record.originalRelation, &record.resultRelation, &record.actorUserID,
		&record.expectedClaimVersion, &record.reasonCode, &record.note, &record.commandFingerprint, &record.createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return claimEvidenceFeedbackRecord{}, false, nil
	}
	return record, err == nil, databaserepository.MapError(err)
}

func (repository *ClaimEvidencePostgresRepository) ReadClaimEvidenceTarget(ctx context.Context, query eventapplication.ClaimEvidenceTargetQuery) (eventapplication.ClaimEvidenceTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.MicroEventID <= 0 || query.DocumentVersionID <= 0 || query.TextQuoteSelectorID <= 0 || query.DecisionAt.IsZero() {
		return eventapplication.ClaimEvidenceTargetDTO{}, eventapplication.ErrInvalidClaimEvidenceContract
	}
	var value eventapplication.ClaimEvidenceTargetDTO
	var sourceURL, canonicalURL, publisherName, originName sql.NullString
	var publisherID, originID sql.NullInt64
	var publishedAt sql.NullTime
	err := repository.queryRow(ctx, `
SELECT event.id,event.version,version.id,selector.id,family.id,family.root_document_version_id,
       selector.quote_sha256,selector.plaintext_sha256,selector.selector_version,selector.retention_until,
       selector.retention_until>$4
       AND current_rights_action_allowed(selector.quote_rights_decision_id,selector.source_connection_id,
           'document_version',version.id::text,selector.plaintext_sha256,'quote',$4)
       AND current_rights_action_allowed(selector.retain_rights_decision_id,selector.source_connection_id,
           'document_version',version.id::text,selector.plaintext_sha256,'retain',$4),
       observation.source_record_url,observation.canonical_url,
       publisher.source_party_id,publisher.display_name_snapshot,
       content_origin.source_party_id,content_origin.display_name_snapshot,
       observation.published_at,observation.captured_at
FROM micro_events AS event
JOIN micro_event_members AS event_member ON event_member.micro_event_id=event.id AND event_member.active
JOIN content_families AS family ON family.id=event_member.content_family_id AND family.status IN ('active','review_pending')
JOIN content_family_members AS family_member ON family_member.family_id=family.id
    AND family_member.document_version_id=$2 AND family_member.active
JOIN document_versions AS version ON version.id=family_member.document_version_id
JOIN source_observations AS observation ON observation.id=version.source_observation_id
JOIN document_text_quote_selectors AS selector ON selector.id=$3 AND selector.document_version_id=version.id
LEFT JOIN source_observation_parties AS publisher ON publisher.source_observation_id=observation.id AND publisher.role='publisher'
LEFT JOIN source_observation_parties AS content_origin ON content_origin.source_observation_id=observation.id AND content_origin.role='content_origin'
WHERE event.id=$1`, query.MicroEventID, query.DocumentVersionID, query.TextQuoteSelectorID, query.DecisionAt.UTC()).Scan(
		&value.MicroEventID, &value.MicroEventVersion, &value.DocumentVersionID, &value.TextQuoteSelectorID,
		&value.ContentFamilyID, &value.LineageRootID, &value.QuoteSHA256, &value.PlaintextSHA256,
		&value.SelectorVersion, &value.SelectorRetentionUntil, &value.CurrentlyCitable,
		&sourceURL, &canonicalURL, &publisherID, &publisherName, &originID, &originName, &publishedAt, &value.CapturedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return eventapplication.ClaimEvidenceTargetDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return eventapplication.ClaimEvidenceTargetDTO{}, databaserepository.MapError(err)
	}
	value.SourceRecordURL, value.CanonicalURL = nullableClaimEvidenceString(sourceURL), nullableClaimEvidenceString(canonicalURL)
	value.PublisherPartyID, value.PublisherName = nullableClaimEvidenceInt64(publisherID), nullableClaimEvidenceString(publisherName)
	value.ContentOriginPartyID, value.ContentOriginName = nullableClaimEvidenceInt64(originID), nullableClaimEvidenceString(originName)
	value.PublishedAt = nullableClaimEvidenceTime(publishedAt)
	value.CapturedAt, value.SelectorRetentionUntil = value.CapturedAt.UTC(), value.SelectorRetentionUntil.UTC()
	return value, nil
}

func (repository *ClaimEvidencePostgresRepository) CommitClaimEvidence(ctx context.Context, command eventapplication.CommitClaimEvidenceCommand) (eventapplication.RecordClaimEvidenceResult, error) {
	if repository == nil || repository.runtime == nil || command.Target.MicroEventID <= 0 || command.Target.MicroEventVersion <= 0 ||
		command.Target.DocumentVersionID <= 0 || command.Target.TextQuoteSelectorID <= 0 || command.Target.ContentFamilyID <= 0 ||
		command.Target.LineageRootID <= 0 || len(command.ClaimHash) != 64 || len(command.CommandFingerprint) != 64 ||
		strings.TrimSpace(command.IdempotencyKey) == "" || command.DecisionAt.IsZero() {
		return eventapplication.RecordClaimEvidenceResult{}, eventapplication.ErrInvalidClaimEvidenceContract
	}
	var result eventapplication.RecordClaimEvidenceResult
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		existingEvidence, found, readErr := readClaimEvidenceByIdempotency(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if readErr != nil {
			return readErr
		}
		if found {
			if existingEvidence.commandFingerprint != command.CommandFingerprint {
				return sharedrepository.ErrConflict
			}
			claim, claimErr := readClaimRecord(transactionCtx, transaction.SQL, existingEvidence.claimID)
			if claimErr != nil {
				return claimErr
			}
			result, readErr = claimEvidenceResult(claim, existingEvidence, false)
			return readErr
		}
		qualifiers, _ := json.Marshal(command.Qualifiers)
		claim, created, claimErr := insertOrReadClaim(transactionCtx, transaction.SQL, command, qualifiers)
		if claimErr != nil {
			return claimErr
		}
		if !claimRecordMatches(claim, command, qualifiers) {
			return sharedrepository.ErrConflict
		}
		evidence, evidenceCreated, evidenceErr := insertOrReadClaimEvidence(transactionCtx, transaction.SQL, claim.id, command)
		if evidenceErr != nil {
			return evidenceErr
		}
		if !claimEvidenceRecordMatches(evidence, claim.id, command) {
			return sharedrepository.ErrConflict
		}
		result, evidenceErr = claimEvidenceResult(claim, evidence, created || evidenceCreated)
		return evidenceErr
	})
	if err != nil {
		return eventapplication.RecordClaimEvidenceResult{}, databaserepository.MapError(err)
	}
	return result, nil
}

func (repository *ClaimEvidencePostgresRepository) ReadEvidenceStateTarget(ctx context.Context, query eventapplication.EvidenceStateTargetQuery) (eventapplication.EvidenceStateTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.MicroEventID <= 0 || query.EventVersion <= 0 ||
		strings.TrimSpace(query.AlgorithmVersion) == "" || query.CalculatedAt.IsZero() {
		return eventapplication.EvidenceStateTargetDTO{}, eventapplication.ErrInvalidClaimEvidenceContract
	}
	var value eventapplication.EvidenceStateTargetDTO
	err := repository.queryRow(ctx, `SELECT event.id,event.version,profile.id,profile.algorithm_version
FROM micro_events AS event CROSS JOIN evidence_state_profiles AS profile
WHERE event.id=$1 AND event.version=$2 AND profile.algorithm_version=$3 AND profile.status='active'`,
		query.MicroEventID, query.EventVersion, query.AlgorithmVersion).Scan(&value.MicroEventID, &value.EventVersion, &value.ProfileID, &value.AlgorithmVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return eventapplication.EvidenceStateTargetDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return eventapplication.EvidenceStateTargetDTO{}, databaserepository.MapError(err)
	}
	rows, err := repository.queryExecutor(ctx).QueryContext(ctx, `
SELECT evidence.id,evidence.lineage_root_document_version_id,evidence.relation,
       selector.retention_until>$2
       AND current_rights_action_allowed(selector.quote_rights_decision_id,selector.source_connection_id,
           'document_version',evidence.document_version_id::text,evidence.plaintext_sha256,'quote',$2)
       AND current_rights_action_allowed(selector.retain_rights_decision_id,selector.source_connection_id,
           'document_version',evidence.document_version_id::text,evidence.plaintext_sha256,'retain',$2)
       AND family.status IN ('active','review_pending')
       AND family_member.active AND event_member.active
FROM claims AS claim
JOIN claim_evidence_versions AS evidence ON evidence.claim_id=claim.id
JOIN document_text_quote_selectors AS selector ON selector.id=evidence.text_quote_selector_id
JOIN content_families AS family ON family.id=evidence.content_family_id
JOIN content_family_members AS family_member ON family_member.family_id=family.id
  AND family_member.document_version_id=evidence.document_version_id
JOIN micro_event_members AS event_member ON event_member.content_family_id=family.id
  AND event_member.micro_event_id=claim.micro_event_id
WHERE claim.micro_event_id=$1 ORDER BY evidence.id`, query.MicroEventID, query.CalculatedAt.UTC())
	if err != nil {
		return eventapplication.EvidenceStateTargetDTO{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	value.Items = []eventapplication.EvidenceStateItemDTO{}
	for rows.Next() {
		var item eventapplication.EvidenceStateItemDTO
		if err := rows.Scan(&item.ClaimEvidenceVersionID, &item.LineageRootID, &item.Relation, &item.Citable); err != nil {
			return eventapplication.EvidenceStateTargetDTO{}, databaserepository.MapError(err)
		}
		value.Items = append(value.Items, item)
	}
	if err := rows.Err(); err != nil {
		return eventapplication.EvidenceStateTargetDTO{}, databaserepository.MapError(err)
	}
	return value, nil
}

func (repository *ClaimEvidencePostgresRepository) CommitEvidenceStateSnapshot(ctx context.Context, command eventapplication.CommitEvidenceStateSnapshotCommand) (eventapplication.EvidenceStateSnapshotDTO, error) {
	if repository == nil || repository.runtime == nil || command.MicroEventID <= 0 || command.EventVersion <= 0 || command.ProfileID <= 0 ||
		len(command.EvidenceSetHash) != 64 || command.IndependentOriginCount < 0 || len(command.ReasonCodes) == 0 || command.CalculatedAt.IsZero() {
		return eventapplication.EvidenceStateSnapshotDTO{}, eventapplication.ErrInvalidClaimEvidenceContract
	}
	ids := append([]int64(nil), command.ClaimEvidenceVersionIDs...)
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for index, id := range ids {
		if id <= 0 || index > 0 && ids[index-1] == id {
			return eventapplication.EvidenceStateSnapshotDTO{}, eventapplication.ErrInvalidClaimEvidenceContract
		}
	}
	var result eventapplication.EvidenceStateSnapshotDTO
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		stored, found, readErr := readEvidenceStateSnapshot(transactionCtx, transaction.SQL, command)
		if readErr != nil {
			return readErr
		}
		if found {
			storedIDs, itemsErr := readEvidenceStateSnapshotItems(transactionCtx, transaction.SQL, stored.id)
			if itemsErr != nil {
				return itemsErr
			}
			result, readErr = stored.dto(storedIDs, false)
			return readErr
		}
		var validCount int
		if len(ids) > 0 {
			if err := transaction.SQL.QueryRowContext(transactionCtx, `SELECT count(*) FROM claim_evidence_versions AS evidence
JOIN claims AS claim ON claim.id=evidence.claim_id WHERE claim.micro_event_id=$1 AND evidence.id=ANY($2)`, command.MicroEventID, ids).Scan(&validCount); err != nil {
				return databaserepository.MapError(err)
			}
			if validCount != len(ids) {
				return sharedrepository.ErrConstraint
			}
		}
		reasons, _ := json.Marshal(command.ReasonCodes)
		stored = evidenceStateSnapshotRecord{}
		if err := transaction.SQL.QueryRowContext(transactionCtx, `INSERT INTO evidence_state_snapshots (
micro_event_id,micro_event_version,evidence_state_profile_id,algorithm_version,evidence_set_hash,evidence_state,
independent_origin_count,reason_codes,calculated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9)
RETURNING id,version,micro_event_id,micro_event_version,evidence_state_profile_id,algorithm_version,evidence_set_hash,
evidence_state,independent_origin_count,reason_codes,calculated_at`, command.MicroEventID, command.EventVersion,
			command.ProfileID, command.AlgorithmVersion, command.EvidenceSetHash, command.State,
			command.IndependentOriginCount, string(reasons), command.CalculatedAt.UTC()).Scan(&stored.id, &stored.version,
			&stored.microEventID, &stored.eventVersion, &stored.profileID, &stored.algorithmVersion, &stored.evidenceSetHash,
			&stored.state, &stored.independentOrigins, &stored.reasonCodesJSON, &stored.calculatedAt); err != nil {
			return databaserepository.MapError(err)
		}
		for ordinal, id := range ids {
			if _, err := transaction.SQL.ExecContext(transactionCtx, `INSERT INTO evidence_state_snapshot_items
(evidence_state_snapshot_id,claim_evidence_version_id,ordinal) VALUES ($1,$2,$3)`, stored.id, id, ordinal); err != nil {
				return databaserepository.MapError(err)
			}
		}
		result, readErr = stored.dto(ids, true)
		return readErr
	})
	if err != nil {
		return eventapplication.EvidenceStateSnapshotDTO{}, databaserepository.MapError(err)
	}
	return result, nil
}

func (repository *ClaimEvidencePostgresRepository) ReadClaimEvidenceCorrectionTarget(ctx context.Context, query eventapplication.ClaimEvidenceCorrectionTargetQuery) (eventapplication.ClaimEvidenceCorrectionTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.OriginalClaimEvidenceVersionID <= 0 || query.ResultTextQuoteSelectorID <= 0 || query.DecisionAt.IsZero() {
		return eventapplication.ClaimEvidenceCorrectionTargetDTO{}, eventapplication.ErrInvalidClaimEvidenceContract
	}
	var evidence claimEvidenceVersionRecord
	if err := scanClaimEvidence(repository.queryRow(ctx, `SELECT `+claimEvidenceColumns+` FROM claim_evidence_versions WHERE id=$1`,
		query.OriginalClaimEvidenceVersionID), &evidence); errors.Is(err, sql.ErrNoRows) {
		return eventapplication.ClaimEvidenceCorrectionTargetDTO{}, sharedrepository.ErrNotFound
	} else if err != nil {
		return eventapplication.ClaimEvidenceCorrectionTargetDTO{}, databaserepository.MapError(err)
	}
	var claim claimRecord
	if err := repository.queryRow(ctx, `SELECT id,version,micro_event_id,micro_event_version,claim_hash,subject,predicate,object,qualifiers,created_at
FROM claims WHERE id=$1`, evidence.claimID).Scan(&claim.id, &claim.version, &claim.microEventID, &claim.microEventVersion,
		&claim.claimHash, &claim.subject, &claim.predicate, &claim.object, &claim.qualifiersJSON, &claim.createdAt); err != nil {
		return eventapplication.ClaimEvidenceCorrectionTargetDTO{}, databaserepository.MapError(err)
	}
	claimDTO, err := claim.dto()
	if err != nil {
		return eventapplication.ClaimEvidenceCorrectionTargetDTO{}, err
	}
	resultTarget, err := repository.ReadClaimEvidenceTarget(ctx, eventapplication.ClaimEvidenceTargetQuery{
		MicroEventID: claim.microEventID, DocumentVersionID: evidence.documentVersionID,
		TextQuoteSelectorID: query.ResultTextQuoteSelectorID, DecisionAt: query.DecisionAt.UTC()})
	if err != nil {
		return eventapplication.ClaimEvidenceCorrectionTargetDTO{}, err
	}
	return eventapplication.ClaimEvidenceCorrectionTargetDTO{Claim: claimDTO, OriginalEvidence: evidence.dto(), ResultTarget: resultTarget}, nil
}

func (repository *ClaimEvidencePostgresRepository) CommitClaimEvidenceCorrection(ctx context.Context, command eventapplication.CommitClaimEvidenceCorrectionCommand) (eventapplication.CorrectClaimEvidenceResult, error) {
	if repository == nil || repository.runtime == nil || command.Target.Claim.ID <= 0 || command.Target.OriginalEvidence.ID <= 0 ||
		command.Target.ResultTarget.TextQuoteSelectorID <= 0 || command.ActorUserID <= 0 || strings.TrimSpace(command.ReasonCode) == "" ||
		strings.TrimSpace(command.IdempotencyKey) == "" || len(command.CommandFingerprint) != 64 || command.DecisionAt.IsZero() {
		return eventapplication.CorrectClaimEvidenceResult{}, eventapplication.ErrInvalidClaimEvidenceContract
	}
	var result eventapplication.CorrectClaimEvidenceResult
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		feedback, found, readErr := readClaimEvidenceFeedback(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if readErr != nil {
			return readErr
		}
		if found {
			if feedback.commandFingerprint != command.CommandFingerprint {
				return sharedrepository.ErrConflict
			}
			var evidence claimEvidenceVersionRecord
			if err := scanClaimEvidence(transaction.SQL.QueryRowContext(transactionCtx, `SELECT `+claimEvidenceColumns+` FROM claim_evidence_versions WHERE id=$1`, feedback.resultEvidenceID), &evidence); err != nil {
				return databaserepository.MapError(err)
			}
			result = eventapplication.CorrectClaimEvidenceResult{Evidence: evidence.dto(), Feedback: feedback.dto(), Created: false}
			return nil
		}
		claim, err := readClaimRecord(transactionCtx, transaction.SQL, command.Target.Claim.ID)
		if err != nil {
			return err
		}
		if claim.version != command.Target.Claim.Version || claim.microEventID != command.Target.Claim.MicroEventID ||
			claim.microEventVersion != command.Target.Claim.MicroEventVersion {
			return sharedrepository.ErrConflict
		}
		var original claimEvidenceVersionRecord
		if err := scanClaimEvidence(transaction.SQL.QueryRowContext(transactionCtx, `SELECT `+claimEvidenceColumns+` FROM claim_evidence_versions WHERE id=$1 FOR KEY SHARE`,
			command.Target.OriginalEvidence.ID), &original); err != nil {
			return databaserepository.MapError(err)
		}
		if original.claimID != claim.id || original.documentVersionID != command.Target.ResultTarget.DocumentVersionID {
			return sharedrepository.ErrConflict
		}
		resultEvidenceKeyDigest := sha256.Sum256([]byte("claim-evidence-correction:" + command.IdempotencyKey))
		resultEvidenceKey := "correction-" + hex.EncodeToString(resultEvidenceKeyDigest[:24])
		evidence, _, err := insertOrReadClaimEvidence(transactionCtx, transaction.SQL, claim.id, eventapplication.CommitClaimEvidenceCommand{
			Target: command.Target.ResultTarget, Relation: command.ResultRelation,
			ExtractionSchemaVersion: original.extractionVersion, Origin: "manual", ActorUserID: &command.ActorUserID,
			IdempotencyKey: resultEvidenceKey, CommandFingerprint: command.CommandFingerprint, DecisionAt: command.DecisionAt,
		})
		if err != nil {
			return err
		}
		if evidence.id == original.id {
			return sharedrepository.ErrConflict
		}
		feedback = claimEvidenceFeedbackRecord{}
		if err := transaction.SQL.QueryRowContext(transactionCtx, `INSERT INTO claim_evidence_feedbacks (
claim_id,original_claim_evidence_version_id,result_claim_evidence_version_id,target_document_version_id,
original_text_quote_selector_id,result_text_quote_selector_id,original_relation,result_relation,actor_user_id,
expected_claim_version,reason_code,note,idempotency_key,command_fingerprint,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
RETURNING id,version,claim_id,original_claim_evidence_version_id,result_claim_evidence_version_id,target_document_version_id,
original_text_quote_selector_id,result_text_quote_selector_id,original_relation,result_relation,actor_user_id,
expected_claim_version,reason_code,note,command_fingerprint,created_at`, claim.id, original.id, evidence.id,
			original.documentVersionID, original.selectorID, evidence.selectorID, original.relation, evidence.relation,
			command.ActorUserID, claim.version, command.ReasonCode, command.Note, command.IdempotencyKey,
			command.CommandFingerprint, command.DecisionAt.UTC()).Scan(&feedback.id, &feedback.version, &feedback.claimID,
			&feedback.originalEvidenceID, &feedback.resultEvidenceID, &feedback.targetDocumentVersionID,
			&feedback.originalSelectorID, &feedback.resultSelectorID, &feedback.originalRelation, &feedback.resultRelation,
			&feedback.actorUserID, &feedback.expectedClaimVersion, &feedback.reasonCode, &feedback.note,
			&feedback.commandFingerprint, &feedback.createdAt); err != nil {
			return databaserepository.MapError(err)
		}
		result = eventapplication.CorrectClaimEvidenceResult{Evidence: evidence.dto(), Feedback: feedback.dto(), Created: true}
		return nil
	})
	if err != nil {
		return eventapplication.CorrectClaimEvidenceResult{}, databaserepository.MapError(err)
	}
	return result, nil
}

func insertOrReadClaim(ctx context.Context, tx *sql.Tx, command eventapplication.CommitClaimEvidenceCommand, qualifiers []byte) (claimRecord, bool, error) {
	var record claimRecord
	err := tx.QueryRowContext(ctx, `INSERT INTO claims (micro_event_id,micro_event_version,claim_hash,subject,predicate,object,qualifiers,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8) ON CONFLICT (micro_event_id,claim_hash) DO NOTHING
RETURNING id,version,micro_event_id,micro_event_version,claim_hash,subject,predicate,object,qualifiers,created_at`,
		command.Target.MicroEventID, command.Target.MicroEventVersion, command.ClaimHash, command.Subject,
		command.Predicate, command.Object, string(qualifiers), command.DecisionAt.UTC()).Scan(&record.id, &record.version,
		&record.microEventID, &record.microEventVersion, &record.claimHash, &record.subject, &record.predicate,
		&record.object, &record.qualifiersJSON, &record.createdAt)
	if err == nil {
		return record, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return claimRecord{}, false, databaserepository.MapError(err)
	}
	err = tx.QueryRowContext(ctx, `SELECT id,version,micro_event_id,micro_event_version,claim_hash,subject,predicate,object,qualifiers,created_at
FROM claims WHERE micro_event_id=$1 AND claim_hash=$2 FOR KEY SHARE`, command.Target.MicroEventID, command.ClaimHash).Scan(
		&record.id, &record.version, &record.microEventID, &record.microEventVersion, &record.claimHash, &record.subject,
		&record.predicate, &record.object, &record.qualifiersJSON, &record.createdAt)
	return record, false, databaserepository.MapError(err)
}

func insertOrReadClaimEvidence(ctx context.Context, tx *sql.Tx, claimID int64, command eventapplication.CommitClaimEvidenceCommand) (claimEvidenceVersionRecord, bool, error) {
	var record claimEvidenceVersionRecord
	arguments := []any{claimID, command.Target.DocumentVersionID, command.Target.TextQuoteSelectorID, command.Target.ContentFamilyID,
		command.Target.LineageRootID, command.Relation, command.Target.QuoteSHA256, command.Target.PlaintextSHA256,
		command.Target.SelectorVersion, command.Target.SourceRecordURL, command.Target.CanonicalURL, command.Target.PublisherPartyID,
		command.Target.PublisherName, command.Target.ContentOriginPartyID, command.Target.ContentOriginName, command.Target.PublishedAt,
		command.Target.CapturedAt, command.ModelRunID, command.ModelRelationScore, command.ExtractionSchemaVersion, command.Origin,
		command.ActorUserID, command.IdempotencyKey, command.CommandFingerprint, command.Target.SelectorRetentionUntil, command.DecisionAt.UTC()}
	err := scanClaimEvidence(tx.QueryRowContext(ctx, `INSERT INTO claim_evidence_versions (
claim_id,document_version_id,text_quote_selector_id,content_family_id,lineage_root_document_version_id,relation,
quote_sha256,plaintext_sha256,selector_version,source_record_url_snapshot,canonical_url_snapshot,publisher_party_id,
publisher_name_snapshot,content_origin_party_id,content_origin_name_snapshot,published_at_snapshot,captured_at_snapshot,
model_run_id,model_relation_score,extraction_schema_version,decision_origin,actor_user_id,idempotency_key,command_fingerprint,
retention_until,created_at) VALUES (`+claimEvidencePlaceholders(26)+`) ON CONFLICT DO NOTHING RETURNING `+claimEvidenceColumns, arguments...), &record)
	if err == nil {
		return record, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return claimEvidenceVersionRecord{}, false, databaserepository.MapError(err)
	}
	err = scanClaimEvidence(tx.QueryRowContext(ctx, `SELECT `+claimEvidenceColumns+` FROM claim_evidence_versions
WHERE claim_id=$1 AND document_version_id=$2 AND quote_sha256=$3 AND relation=$4 AND extraction_schema_version=$5 FOR KEY SHARE`,
		claimID, command.Target.DocumentVersionID, command.Target.QuoteSHA256, command.Relation, command.ExtractionSchemaVersion), &record)
	return record, false, databaserepository.MapError(err)
}

const claimEvidenceColumns = `id,version,claim_id,document_version_id,text_quote_selector_id,content_family_id,lineage_root_document_version_id,
relation,quote_sha256,plaintext_sha256,selector_version,source_record_url_snapshot,canonical_url_snapshot,publisher_party_id,
publisher_name_snapshot,content_origin_party_id,content_origin_name_snapshot,published_at_snapshot,captured_at_snapshot,
model_run_id,model_relation_score,extraction_schema_version,decision_origin,actor_user_id,retention_until,created_at`

func claimEvidencePlaceholders(count int) string {
	parts := make([]string, count)
	for index := range parts {
		parts[index] = fmt.Sprintf("$%d", index+1)
	}
	return strings.Join(parts, ",")
}

func scanClaimEvidence(row *sql.Row, record *claimEvidenceVersionRecord) error {
	return row.Scan(&record.id, &record.version, &record.claimID, &record.documentVersionID, &record.selectorID,
		&record.familyID, &record.lineageRootID, &record.relation, &record.quoteSHA, &record.plaintextSHA,
		&record.selectorVersion, &record.sourceRecordURL, &record.canonicalURL, &record.publisherPartyID,
		&record.publisherName, &record.contentOriginPartyID, &record.contentOriginName, &record.publishedAt,
		&record.capturedAt, &record.modelRunID, &record.modelRelationScore, &record.extractionVersion,
		&record.origin, &record.actorUserID, &record.retentionUntil, &record.createdAt)
}

type storedClaimEvidence struct {
	claimEvidenceVersionRecord
	commandFingerprint string
}

func readClaimEvidenceByIdempotency(ctx context.Context, tx *sql.Tx, key string) (storedClaimEvidence, bool, error) {
	var value storedClaimEvidence
	err := scanClaimEvidenceWithFingerprint(tx.QueryRowContext(ctx, `SELECT `+claimEvidenceColumns+`,command_fingerprint
FROM claim_evidence_versions WHERE idempotency_key=$1 FOR KEY SHARE`, key), &value)
	if errors.Is(err, sql.ErrNoRows) {
		return storedClaimEvidence{}, false, nil
	}
	return value, err == nil, databaserepository.MapError(err)
}
func scanClaimEvidenceWithFingerprint(row *sql.Row, value *storedClaimEvidence) error {
	return row.Scan(&value.id, &value.version, &value.claimID, &value.documentVersionID, &value.selectorID,
		&value.familyID, &value.lineageRootID, &value.relation, &value.quoteSHA, &value.plaintextSHA,
		&value.selectorVersion, &value.sourceRecordURL, &value.canonicalURL, &value.publisherPartyID,
		&value.publisherName, &value.contentOriginPartyID, &value.contentOriginName, &value.publishedAt,
		&value.capturedAt, &value.modelRunID, &value.modelRelationScore, &value.extractionVersion,
		&value.origin, &value.actorUserID, &value.retentionUntil, &value.createdAt, &value.commandFingerprint)
}

func readClaimRecord(ctx context.Context, tx *sql.Tx, id int64) (claimRecord, error) {
	var record claimRecord
	err := tx.QueryRowContext(ctx, `SELECT id,version,micro_event_id,micro_event_version,claim_hash,subject,predicate,object,qualifiers,created_at
FROM claims WHERE id=$1`, id).Scan(&record.id, &record.version, &record.microEventID, &record.microEventVersion,
		&record.claimHash, &record.subject, &record.predicate, &record.object, &record.qualifiersJSON, &record.createdAt)
	return record, databaserepository.MapError(err)
}
func claimEvidenceResult(claim claimRecord, evidence interface {
	dto() eventapplication.ClaimEvidenceVersionDTO
}, created bool) (eventapplication.RecordClaimEvidenceResult, error) {
	claimDTO, err := claim.dto()
	if err != nil {
		return eventapplication.RecordClaimEvidenceResult{}, err
	}
	return eventapplication.RecordClaimEvidenceResult{Claim: claimDTO, Evidence: evidence.dto(), Created: created}, nil
}

func (record storedClaimEvidence) dto() eventapplication.ClaimEvidenceVersionDTO {
	return record.claimEvidenceVersionRecord.dto()
}

func claimRecordMatches(record claimRecord, command eventapplication.CommitClaimEvidenceCommand, qualifiers []byte) bool {
	var left, right any
	return record.microEventID == command.Target.MicroEventID && record.microEventVersion == command.Target.MicroEventVersion &&
		strings.TrimSpace(record.claimHash) == command.ClaimHash && record.subject == command.Subject && record.predicate == command.Predicate &&
		record.object == command.Object && json.Unmarshal(record.qualifiersJSON, &left) == nil && json.Unmarshal(qualifiers, &right) == nil && fmt.Sprint(left) == fmt.Sprint(right)
}
func claimEvidenceRecordMatches(record claimEvidenceVersionRecord, claimID int64, command eventapplication.CommitClaimEvidenceCommand) bool {
	return record.claimID == claimID && record.documentVersionID == command.Target.DocumentVersionID && record.selectorID == command.Target.TextQuoteSelectorID &&
		record.familyID == command.Target.ContentFamilyID && record.lineageRootID == command.Target.LineageRootID && record.relation == command.Relation &&
		strings.TrimSpace(record.quoteSHA) == command.Target.QuoteSHA256 && strings.TrimSpace(record.plaintextSHA) == command.Target.PlaintextSHA256 &&
		record.selectorVersion == command.Target.SelectorVersion && record.extractionVersion == command.ExtractionSchemaVersion && record.origin == command.Origin
}

func readEvidenceStateSnapshot(ctx context.Context, tx *sql.Tx, command eventapplication.CommitEvidenceStateSnapshotCommand) (evidenceStateSnapshotRecord, bool, error) {
	var record evidenceStateSnapshotRecord
	err := tx.QueryRowContext(ctx, `SELECT id,version,micro_event_id,micro_event_version,evidence_state_profile_id,
algorithm_version,evidence_set_hash,evidence_state,independent_origin_count,reason_codes,calculated_at
FROM evidence_state_snapshots WHERE micro_event_id=$1 AND micro_event_version=$2 AND evidence_state_profile_id=$3 AND evidence_set_hash=$4 FOR KEY SHARE`,
		command.MicroEventID, command.EventVersion, command.ProfileID, command.EvidenceSetHash).Scan(&record.id, &record.version,
		&record.microEventID, &record.eventVersion, &record.profileID, &record.algorithmVersion, &record.evidenceSetHash,
		&record.state, &record.independentOrigins, &record.reasonCodesJSON, &record.calculatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return evidenceStateSnapshotRecord{}, false, nil
	}
	return record, err == nil, databaserepository.MapError(err)
}
func readEvidenceStateSnapshotItems(ctx context.Context, tx *sql.Tx, snapshotID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT claim_evidence_version_id FROM evidence_state_snapshot_items WHERE evidence_state_snapshot_id=$1 ORDER BY ordinal`, snapshotID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, databaserepository.MapError(err)
		}
		ids = append(ids, id)
	}
	return ids, databaserepository.MapError(rows.Err())
}

type claimEvidenceQueryExecutor interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *ClaimEvidencePostgresRepository) queryExecutor(ctx context.Context) claimEvidenceQueryExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}
func (repository *ClaimEvidencePostgresRepository) queryRow(ctx context.Context, query string, arguments ...any) *sql.Row {
	return repository.queryExecutor(ctx).QueryRowContext(ctx, query, arguments...)
}
func (repository *ClaimEvidencePostgresRepository) withTransaction(ctx context.Context, operation func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return operation(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, operation)
}
