package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var _ ingestionapplication.ContentLineageFeedbackRepository = (*ContentFamilyRepository)(nil)

func (repository *ContentFamilyRepository) FindContentLineageFeedbackReceipt(ctx context.Context, query ingestionapplication.FindContentLineageFeedbackReceiptQuery) (ingestionapplication.ContentLineageFeedbackDTO, bool, error) {
	if repository == nil || repository.runtime == nil || query.ActorUserID <= 0 || strings.TrimSpace(query.IdempotencyKey) == "" ||
		len(query.IdempotencyKey) > 96 || !validContentFamilySHA(query.CommandFingerprint) {
		return ingestionapplication.ContentLineageFeedbackDTO{}, false, ingestionapplication.ErrInvalidContentFamilyContract
	}
	var record contentLineageFeedbackRecord
	err := repository.contentLineageQueryRow(ctx, `
SELECT feedback.id,feedback.lineage_decision_id,feedback.result_lineage_decision_id,feedback.document_version_id,
       feedback.original_family_id,feedback.result_family_id,feedback.result_family_version,feedback.original_relation,
       feedback.result_relation,feedback.original_parent_document_version_id,feedback.result_parent_document_version_id,
       feedback.feedback_type,feedback.actor_user_id,btrim(feedback.command_fingerprint)
FROM content_lineage_feedbacks AS feedback
WHERE feedback.idempotency_key=$1
  AND EXISTS (SELECT 1 FROM users WHERE id=$2 AND role IN ('editor','admin') AND status='active' AND deleted_at IS NULL)`,
		query.IdempotencyKey, query.ActorUserID).Scan(&record.feedbackID, &record.lineageDecisionID,
		&record.resultLineageDecisionID, &record.documentVersionID, &record.originalFamilyID, &record.resultFamilyID,
		&record.resultFamilyVersion, &record.originalRelation, &record.resultRelation, &record.originalParent,
		&record.resultParent, &record.feedbackType, &record.actorUserID, &record.commandFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.ContentLineageFeedbackDTO{}, false, nil
	}
	if err != nil {
		return ingestionapplication.ContentLineageFeedbackDTO{}, false, databaserepository.MapError(err)
	}
	if record.actorUserID != query.ActorUserID {
		return ingestionapplication.ContentLineageFeedbackDTO{}, false, ingestionapplication.ErrContentLineageFeedbackDenied
	}
	if record.commandFingerprint != query.CommandFingerprint {
		return ingestionapplication.ContentLineageFeedbackDTO{}, false, sharedrepository.ErrConflict
	}
	return record.dto(true), true, nil
}

func (repository *ContentFamilyRepository) ReadContentLineageFeedbackTarget(ctx context.Context, query ingestionapplication.ReadContentLineageFeedbackTargetQuery) (ingestionapplication.ContentLineageFeedbackTargetDTO, error) {
	if repository == nil || repository.runtime == nil || query.ActorUserID <= 0 || query.LineageDecisionID <= 0 {
		return ingestionapplication.ContentLineageFeedbackTargetDTO{}, ingestionapplication.ErrInvalidContentFamilyContract
	}
	var record contentLineageFeedbackTargetRecord
	err := repository.contentLineageQueryRow(ctx, `
SELECT decision.id,decision.document_version_id,decision.fingerprint_id,member.family_id,family.version,
       member.id,member.version,member.relation,member.parent_document_version_id,member.lineage_profile_version
FROM content_lineage_decisions AS decision
JOIN content_family_members AS member ON member.lineage_decision_id=decision.id AND member.active
JOIN content_families AS family ON family.id=member.family_id AND family.status IN ('active','review_pending')
WHERE decision.id=$1
  AND EXISTS (SELECT 1 FROM users WHERE id=$2 AND role IN ('editor','admin') AND status='active' AND deleted_at IS NULL)`,
		query.LineageDecisionID, query.ActorUserID).Scan(&record.lineageDecisionID, &record.documentVersionID,
		&record.fingerprintID, &record.familyID, &record.familyVersion, &record.memberID, &record.memberVersion,
		&record.relation, &record.parentDocumentVersionID, &record.profileVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.ContentLineageFeedbackTargetDTO{}, ingestionapplication.ErrContentLineageFeedbackDenied
	}
	if err != nil {
		return ingestionapplication.ContentLineageFeedbackTargetDTO{}, databaserepository.MapError(err)
	}
	return record.dto(), nil
}

func (repository *ContentFamilyRepository) CommitContentLineageFeedback(ctx context.Context, command ingestionapplication.CommitContentLineageFeedbackCommand) (ingestionapplication.ContentLineageFeedbackDTO, error) {
	if repository == nil || repository.runtime == nil || validateContentLineageFeedbackCommit(command) != nil {
		return ingestionapplication.ContentLineageFeedbackDTO{}, ingestionapplication.ErrInvalidContentFamilyContract
	}
	var result ingestionapplication.ContentLineageFeedbackDTO
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		replayed, found, replayErr := readContentLineageFeedback(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if replayErr != nil {
			return replayErr
		}
		if found {
			if replayed.commandFingerprint != command.CommandFingerprint {
				return sharedrepository.ErrConflict
			}
			result = replayed.dto(true)
			return nil
		}
		if err := authorizeContentLineageReviewer(transactionCtx, transaction.SQL, command.ActorUserID); err != nil {
			return err
		}
		current, err := lockContentLineageFeedbackTarget(transactionCtx, transaction.SQL, command.LineageDecisionID)
		if err != nil {
			return err
		}
		if !contentLineageTargetMatchesCommand(current, command) {
			return sharedrepository.ErrConflict
		}
		resultFamilyID, resultFamilyVersion, resultRootDocumentVersionID, err := resolveContentLineageFeedbackFamily(transactionCtx, transaction.SQL, command)
		if err != nil {
			return err
		}
		resultDecisionID, err := insertManualContentLineageDecision(transactionCtx, transaction.SQL, command,
			resultFamilyID, resultFamilyVersion, resultRootDocumentVersionID)
		if err != nil {
			return err
		}
		if _, err := transaction.SQL.ExecContext(transactionCtx, `
UPDATE content_family_members SET version=version+1,active=false,retired_at=CURRENT_TIMESTAMP
WHERE id=$1 AND version=$2 AND active`, command.MemberID, command.ExpectedMemberVersion); err != nil {
			return databaserepository.MapError(err)
		}
		var parent any
		if command.ResultParentDocumentVersionID != nil {
			parent = *command.ResultParentDocumentVersionID
		}
		if _, err := transaction.SQL.ExecContext(transactionCtx, `
INSERT INTO content_family_members (
 family_id,document_version_id,fingerprint_id,lineage_decision_id,lineage_profile_version,relation,parent_document_version_id
) VALUES ($1,$2,$3,$4,$5,$6,$7)`, resultFamilyID, command.DocumentVersionID, command.FingerprintID,
			resultDecisionID, command.LineageProfileVersion, command.ResultRelation, parent); err != nil {
			return databaserepository.MapError(err)
		}
		if err := recomputeContentFamilyRoot(transactionCtx, transaction.SQL, command.OriginalFamilyID); err != nil {
			return err
		}
		if resultFamilyID != command.OriginalFamilyID {
			if err := recomputeContentFamilyRoot(transactionCtx, transaction.SQL, resultFamilyID); err != nil {
				return err
			}
		}
		feedbackID, err := insertContentLineageFeedback(transactionCtx, transaction.SQL, command, resultDecisionID,
			resultFamilyID, resultFamilyVersion)
		if err != nil {
			return err
		}
		stored, found, err := readContentLineageFeedbackByID(transactionCtx, transaction.SQL, feedbackID)
		if err != nil || !found {
			return err
		}
		result = stored.dto(false)
		return nil
	})
	if err != nil {
		return ingestionapplication.ContentLineageFeedbackDTO{}, err
	}
	return result, nil
}

func resolveContentLineageFeedbackFamily(ctx context.Context, transaction *sql.Tx, command ingestionapplication.CommitContentLineageFeedbackCommand) (int64, int64, int64, error) {
	if command.ResultRelation == "unrelated" {
		var familyID, version int64
		err := transaction.QueryRowContext(ctx, `
INSERT INTO content_families (root_document_version_id,lineage_profile_version,status)
VALUES ($1,$2,'active') RETURNING id,version`, command.DocumentVersionID, command.LineageProfileVersion).Scan(&familyID, &version)
		if err != nil {
			return 0, 0, 0, databaserepository.MapError(err)
		}
		if _, err := transaction.ExecContext(ctx, `UPDATE content_families SET version=version+1,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND version=$2`, command.OriginalFamilyID, command.ExpectedFamilyVersion); err != nil {
			return 0, 0, 0, databaserepository.MapError(err)
		}
		return familyID, version, command.DocumentVersionID, nil
	}
	if command.ResultParentDocumentVersionID == nil {
		return 0, 0, 0, ingestionapplication.ErrInvalidContentFamilyContract
	}
	var targetMemberID, targetMemberVersion, targetFamilyID, targetFamilyVersion, targetRoot int64
	err := transaction.QueryRowContext(ctx, `
SELECT member.id,member.version,member.family_id,family.version,family.root_document_version_id
FROM content_family_members AS member JOIN content_families AS family ON family.id=member.family_id
WHERE member.document_version_id=$1 AND member.lineage_profile_version=$2 AND member.active
  AND family.status IN ('active','review_pending') FOR UPDATE OF member,family`,
		*command.ResultParentDocumentVersionID, command.LineageProfileVersion).Scan(&targetMemberID, &targetMemberVersion,
		&targetFamilyID, &targetFamilyVersion, &targetRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, sharedrepository.ErrNotFound
	}
	if err != nil {
		return 0, 0, 0, databaserepository.MapError(err)
	}
	if targetMemberVersion != command.ExpectedTargetMemberVersion || *command.ResultParentDocumentVersionID == command.DocumentVersionID {
		return 0, 0, 0, sharedrepository.ErrConflict
	}
	var cycle bool
	err = transaction.QueryRowContext(ctx, `
WITH RECURSIVE ancestors(document_version_id,parent_document_version_id) AS (
  SELECT document_version_id,parent_document_version_id FROM content_family_members
  WHERE document_version_id=$1 AND lineage_profile_version=$2 AND active
  UNION ALL
  SELECT parent.document_version_id,parent.parent_document_version_id
  FROM content_family_members AS parent JOIN ancestors ON parent.document_version_id=ancestors.parent_document_version_id
  WHERE parent.lineage_profile_version=$2 AND parent.active
)
SELECT EXISTS (SELECT 1 FROM ancestors WHERE document_version_id=$3)`, *command.ResultParentDocumentVersionID,
		command.LineageProfileVersion, command.DocumentVersionID).Scan(&cycle)
	if err != nil {
		return 0, 0, 0, databaserepository.MapError(err)
	}
	if cycle {
		return 0, 0, 0, sharedrepository.ErrConflict
	}
	if targetFamilyID == command.OriginalFamilyID {
		if targetFamilyVersion != command.ExpectedFamilyVersion {
			return 0, 0, 0, sharedrepository.ErrConflict
		}
		if _, err := transaction.ExecContext(ctx, `UPDATE content_families SET version=version+1,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND version=$2`, targetFamilyID, targetFamilyVersion); err != nil {
			return 0, 0, 0, databaserepository.MapError(err)
		}
		return targetFamilyID, targetFamilyVersion + 1, targetRoot, nil
	}
	if _, err := transaction.ExecContext(ctx, `UPDATE content_families SET version=version+1,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND version=$2`, command.OriginalFamilyID, command.ExpectedFamilyVersion); err != nil {
		return 0, 0, 0, databaserepository.MapError(err)
	}
	if _, err := transaction.ExecContext(ctx, `UPDATE content_families SET version=version+1,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND version=$2`, targetFamilyID, targetFamilyVersion); err != nil {
		return 0, 0, 0, databaserepository.MapError(err)
	}
	return targetFamilyID, targetFamilyVersion + 1, targetRoot, nil
}

func insertManualContentLineageDecision(ctx context.Context, transaction *sql.Tx, command ingestionapplication.CommitContentLineageFeedbackCommand, familyID, familyVersion, rootDocumentVersionID int64) (int64, error) {
	action := "join"
	var candidateRoot any = rootDocumentVersionID
	if command.ResultRelation == "unrelated" {
		action = "create"
		candidateRoot = nil
	}
	reasons, _ := json.Marshal([]string{"manual_lineage_feedback", command.FeedbackType})
	decisionKey := "lineage-result-" + command.CommandFingerprint
	var decisionID int64
	err := transaction.QueryRowContext(ctx, `
INSERT INTO content_lineage_decisions (
 document_version_id,fingerprint_id,family_id,result_family_version,candidate_root_document_version_id,
 action,relation,hamming_distance,minhash_similarity,decision_profile_version,reason_codes,
 decision_origin,decided_by_user_id,idempotency_key,command_fingerprint
) VALUES ($1,$2,$3,$4,$5,$6,$7,64,0,$8,$9::jsonb,'manual',$10,$11,$12)
RETURNING id`, command.DocumentVersionID, command.FingerprintID, familyID, familyVersion, candidateRoot,
		action, command.ResultRelation, command.LineageProfileVersion, string(reasons), command.ActorUserID,
		decisionKey, command.CommandFingerprint).Scan(&decisionID)
	if err != nil {
		return 0, databaserepository.MapError(err)
	}
	return decisionID, nil
}

func insertContentLineageFeedback(ctx context.Context, transaction *sql.Tx, command ingestionapplication.CommitContentLineageFeedbackCommand, resultDecisionID, resultFamilyID, resultFamilyVersion int64) (int64, error) {
	var override any
	if command.RelationOverride != nil {
		override = *command.RelationOverride
	}
	var originalParent, resultParent any
	if command.OriginalParentDocumentVersionID != nil {
		originalParent = *command.OriginalParentDocumentVersionID
	}
	if command.ResultParentDocumentVersionID != nil {
		resultParent = *command.ResultParentDocumentVersionID
	}
	var id int64
	err := transaction.QueryRowContext(ctx, `
INSERT INTO content_lineage_feedbacks (
 lineage_decision_id,result_lineage_decision_id,document_version_id,original_family_id,result_family_id,
 original_relation,result_relation,original_parent_document_version_id,result_parent_document_version_id,
 result_family_version,actor_user_id,feedback_type,relation_override,reason_code,note,idempotency_key,command_fingerprint
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING id`,
		command.LineageDecisionID, resultDecisionID, command.DocumentVersionID, command.OriginalFamilyID, resultFamilyID,
		command.OriginalRelation, command.ResultRelation, originalParent, resultParent, resultFamilyVersion,
		command.ActorUserID, command.FeedbackType, override, command.ReasonCode, command.Note,
		command.IdempotencyKey, command.CommandFingerprint).Scan(&id)
	if err != nil {
		return 0, databaserepository.MapError(err)
	}
	return id, nil
}

func recomputeContentFamilyRoot(ctx context.Context, transaction *sql.Tx, familyID int64) error {
	var root sql.NullInt64
	if err := transaction.QueryRowContext(ctx, `SELECT min(document_version_id) FROM content_family_members WHERE family_id=$1 AND active`, familyID).Scan(&root); err != nil {
		return databaserepository.MapError(err)
	}
	if root.Valid {
		_, err := transaction.ExecContext(ctx, `UPDATE content_families SET root_document_version_id=$2,status='active',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, familyID, root.Int64)
		return databaserepository.MapError(err)
	}
	_, err := transaction.ExecContext(ctx, `UPDATE content_families SET status='closed',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, familyID)
	return databaserepository.MapError(err)
}

func authorizeContentLineageReviewer(ctx context.Context, transaction *sql.Tx, actorUserID int64) error {
	var allowed bool
	if err := transaction.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id=$1 AND role IN ('editor','admin') AND status='active' AND deleted_at IS NULL)`, actorUserID).Scan(&allowed); err != nil {
		return databaserepository.MapError(err)
	}
	if !allowed {
		return ingestionapplication.ErrContentLineageFeedbackDenied
	}
	return nil
}

func lockContentLineageFeedbackTarget(ctx context.Context, transaction *sql.Tx, decisionID int64) (contentLineageFeedbackTargetRecord, error) {
	var record contentLineageFeedbackTargetRecord
	err := transaction.QueryRowContext(ctx, `
SELECT decision.id,decision.document_version_id,decision.fingerprint_id,member.family_id,family.version,
       member.id,member.version,member.relation,member.parent_document_version_id,member.lineage_profile_version
FROM content_lineage_decisions AS decision
JOIN content_family_members AS member ON member.lineage_decision_id=decision.id AND member.active
JOIN content_families AS family ON family.id=member.family_id AND family.status IN ('active','review_pending')
WHERE decision.id=$1 FOR UPDATE OF member,family`, decisionID).Scan(&record.lineageDecisionID,
		&record.documentVersionID, &record.fingerprintID, &record.familyID, &record.familyVersion,
		&record.memberID, &record.memberVersion, &record.relation, &record.parentDocumentVersionID, &record.profileVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return contentLineageFeedbackTargetRecord{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return contentLineageFeedbackTargetRecord{}, databaserepository.MapError(err)
	}
	return record, nil
}

func contentLineageTargetMatchesCommand(record contentLineageFeedbackTargetRecord, command ingestionapplication.CommitContentLineageFeedbackCommand) bool {
	return record.lineageDecisionID == command.LineageDecisionID && record.documentVersionID == command.DocumentVersionID &&
		record.fingerprintID == command.FingerprintID && record.familyID == command.OriginalFamilyID &&
		record.familyVersion == command.ExpectedFamilyVersion && record.memberID == command.MemberID &&
		record.memberVersion == command.ExpectedMemberVersion && record.relation == command.OriginalRelation &&
		record.profileVersion == command.LineageProfileVersion && nullableLineageParentMatches(record.parentDocumentVersionID, command.OriginalParentDocumentVersionID)
}

func validateContentLineageFeedbackCommit(command ingestionapplication.CommitContentLineageFeedbackCommand) error {
	if command.ActorUserID <= 0 || command.LineageDecisionID <= 0 || command.DocumentVersionID <= 0 || command.FingerprintID <= 0 ||
		command.OriginalFamilyID <= 0 || command.ExpectedFamilyVersion <= 0 || command.MemberID <= 0 || command.ExpectedMemberVersion <= 0 ||
		!validContentFamilyRelation(command.OriginalRelation) || !validContentFamilyRelation(command.ResultRelation) ||
		strings.TrimSpace(command.LineageProfileVersion) == "" || !validContentLineageFeedbackType(command.FeedbackType) ||
		strings.TrimSpace(command.ReasonCode) == "" || len(command.Note) > 1000 || strings.TrimSpace(command.IdempotencyKey) == "" ||
		len(command.IdempotencyKey) > 96 || !validContentFamilySHA(command.CommandFingerprint) {
		return ingestionapplication.ErrInvalidContentFamilyContract
	}
	if command.ResultRelation == "unrelated" {
		if command.ResultParentDocumentVersionID != nil || command.ExpectedTargetMemberVersion != 0 {
			return ingestionapplication.ErrInvalidContentFamilyContract
		}
	} else if command.ResultParentDocumentVersionID == nil || command.ExpectedTargetMemberVersion <= 0 ||
		*command.ResultParentDocumentVersionID == command.DocumentVersionID {
		return ingestionapplication.ErrInvalidContentFamilyContract
	}
	return nil
}

func validContentLineageFeedbackType(value string) bool {
	return value == "duplicate" || value == "not_duplicate" || value == "relation_override" || value == "withdraw"
}

func validContentFamilyRelation(value string) bool {
	return value == "exact_copy" || value == "near_duplicate" || value == "syndicated_from" ||
		value == "translation_of" || value == "revision_of" || value == "unrelated"
}

type contentLineageFeedbackTargetRecord struct {
	lineageDecisionID, documentVersionID, fingerprintID, familyID, familyVersion int64
	memberID, memberVersion                                                      int64
	relation, profileVersion                                                     string
	parentDocumentVersionID                                                      sql.NullInt64
}

func (record contentLineageFeedbackTargetRecord) dto() ingestionapplication.ContentLineageFeedbackTargetDTO {
	return ingestionapplication.ContentLineageFeedbackTargetDTO{LineageDecisionID: record.lineageDecisionID,
		DocumentVersionID: record.documentVersionID, FingerprintID: record.fingerprintID, ContentFamilyID: record.familyID,
		FamilyVersion: record.familyVersion, MemberID: record.memberID, MemberVersion: record.memberVersion,
		Relation: record.relation, ParentDocumentVersionID: nullableLineageParent(record.parentDocumentVersionID),
		LineageProfileVersion: record.profileVersion}
}

type contentLineageFeedbackRecord struct {
	feedbackID, lineageDecisionID, resultLineageDecisionID, documentVersionID int64
	originalFamilyID, resultFamilyID, resultFamilyVersion, actorUserID        int64
	originalRelation, resultRelation, feedbackType, commandFingerprint        string
	originalParent, resultParent                                              sql.NullInt64
}

func (record contentLineageFeedbackRecord) dto(reused bool) ingestionapplication.ContentLineageFeedbackDTO {
	return ingestionapplication.ContentLineageFeedbackDTO{FeedbackID: record.feedbackID,
		LineageDecisionID: record.lineageDecisionID, ResultLineageDecisionID: record.resultLineageDecisionID,
		DocumentVersionID: record.documentVersionID, OriginalFamilyID: record.originalFamilyID,
		ResultFamilyID: record.resultFamilyID, ResultFamilyVersion: record.resultFamilyVersion,
		OriginalRelation: record.originalRelation, ResultRelation: record.resultRelation,
		OriginalParentDocumentVersionID: nullableLineageParent(record.originalParent),
		ResultParentDocumentVersionID:   nullableLineageParent(record.resultParent),
		FeedbackType:                    record.feedbackType, ActorUserID: record.actorUserID, Reused: reused}
}

func readContentLineageFeedback(ctx context.Context, transaction *sql.Tx, key string) (contentLineageFeedbackRecord, bool, error) {
	var id int64
	err := transaction.QueryRowContext(ctx, `SELECT id FROM content_lineage_feedbacks WHERE idempotency_key=$1 FOR UPDATE`, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return contentLineageFeedbackRecord{}, false, nil
	}
	if err != nil {
		return contentLineageFeedbackRecord{}, false, databaserepository.MapError(err)
	}
	return readContentLineageFeedbackByID(ctx, transaction, id)
}

func readContentLineageFeedbackByID(ctx context.Context, transaction *sql.Tx, id int64) (contentLineageFeedbackRecord, bool, error) {
	var record contentLineageFeedbackRecord
	err := transaction.QueryRowContext(ctx, `
SELECT id,lineage_decision_id,result_lineage_decision_id,document_version_id,original_family_id,result_family_id,
       result_family_version,original_relation,result_relation,original_parent_document_version_id,
       result_parent_document_version_id,feedback_type,actor_user_id,btrim(command_fingerprint)
FROM content_lineage_feedbacks WHERE id=$1`, id).Scan(&record.feedbackID, &record.lineageDecisionID,
		&record.resultLineageDecisionID, &record.documentVersionID, &record.originalFamilyID, &record.resultFamilyID,
		&record.resultFamilyVersion, &record.originalRelation, &record.resultRelation, &record.originalParent,
		&record.resultParent, &record.feedbackType, &record.actorUserID, &record.commandFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return contentLineageFeedbackRecord{}, false, nil
	}
	if err != nil {
		return contentLineageFeedbackRecord{}, false, databaserepository.MapError(err)
	}
	return record, true, nil
}

func nullableLineageParent(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullableLineageParentMatches(value sql.NullInt64, expected *int64) bool {
	return !value.Valid && expected == nil || value.Valid && expected != nil && value.Int64 == *expected
}

func (repository *ContentFamilyRepository) contentLineageQueryRow(ctx context.Context, query string, arguments ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, arguments...)
	}
	return repository.runtime.SQL.QueryRowContext(ctx, query, arguments...)
}
