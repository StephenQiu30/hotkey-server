package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"strconv"
	"time"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type projectionRecoveryVault interface {
	Inspect(context.Context, int64) (knowledgeapplication.VaultRecoveryInspection, error)
}

type ProjectionRecoveryRepository struct {
	runtime *database.Runtime
	vault   projectionRecoveryVault
	jobs    *queue.Store
	now     func() time.Time
}

type vaultRecoveryTarget struct {
	documentID int64
	version    int64
	inputHash  string
}

type searchRebuildTarget struct {
	evidenceReferenceID int64
	documentVersionID   int64
	inputHash           string
}

type projectionRecoveryCatalog struct {
	inspection    operationsapplication.ProjectionRecoveryInspectionDTO
	vaultTargets  []vaultRecoveryTarget
	searchTargets []searchRebuildTarget
}

func NewProjectionRecoveryRepository(runtime *database.Runtime, vault projectionRecoveryVault, jobs *queue.Store) (*ProjectionRecoveryRepository, error) {
	if runtime == nil || runtime.SQL == nil || vault == nil || jobs == nil {
		return nil, fmt.Errorf("%w: projection recovery dependencies are required", sharedrepository.ErrUnavailable)
	}
	return &ProjectionRecoveryRepository{runtime: runtime, vault: vault, jobs: jobs, now: time.Now}, nil
}

func (repository *ProjectionRecoveryRepository) InspectProjectionRecovery(ctx context.Context) (operationsapplication.ProjectionRecoveryInspectionDTO, error) {
	catalog, err := repository.inspect(ctx)
	if err != nil {
		return operationsapplication.ProjectionRecoveryInspectionDTO{}, err
	}
	return catalog.inspection, nil
}

func (repository *ProjectionRecoveryRepository) ApplyProjectionRecovery(ctx context.Context, command operationsapplication.ApplyProjectionRecoveryCommand) (operationsapplication.ProjectionRecoveryReceiptDTO, error) {
	if repository == nil || repository.runtime == nil || repository.jobs == nil || repository.vault == nil {
		return operationsapplication.ProjectionRecoveryReceiptDTO{}, sharedrepository.ErrUnavailable
	}
	var receipt operationsapplication.ProjectionRecoveryReceiptDTO
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		if _, err := transaction.SQL.ExecContext(transactionCtx, `SELECT pg_advisory_xact_lock(hashtext('projection-recovery-v1'))`); err != nil {
			return databaserepository.MapError(err)
		}
		before, err := repository.inspect(transactionCtx)
		if err != nil {
			return err
		}
		if !projectionRecoveryCatalogMatchesCommand(before, command) {
			return fmt.Errorf("%w: projection recovery catalog changed", sharedrepository.ErrConflict)
		}
		result, err := transaction.SQL.ExecContext(transactionCtx, `
DELETE FROM notification_delivery_claims
WHERE dispatch_started_at IS NULL`)
		if err != nil {
			return databaserepository.MapError(err)
		}
		removed, err := result.RowsAffected()
		if err != nil {
			return databaserepository.MapError(err)
		}
		if removed != command.ExpectedDisposableClaimCount {
			return fmt.Errorf("%w: disposable delivery claim set changed", sharedrepository.ErrConflict)
		}
		for _, target := range before.vaultTargets {
			_, _, err := repository.jobs.Enqueue(transactionCtx, queue.Job{
				Kind:        queue.KindProjectKnowledge,
				UniqueKey:   queue.StableJobHash(queue.KindProjectKnowledge, "recovery", command.RunSHA256, strconv.FormatInt(target.documentID, 10), target.inputHash),
				Payload:     queue.Payload{EntityID: target.documentID, EntityVersion: target.version, InputHash: target.inputHash},
				ScheduledAt: repository.now().UTC(), MaxAttempts: 5, Priority: 2,
			})
			if err != nil {
				return fmt.Errorf("enqueue Vault recovery: %w", err)
			}
		}
		for _, target := range before.searchTargets {
			args, err := json.Marshal(struct {
				EvidenceReferenceID int64  `json:"evidence_reference_id"`
				TraceID             string `json:"trace_id"`
			}{EvidenceReferenceID: target.evidenceReferenceID})
			if err != nil {
				return errors.New("encode search rebuild identity")
			}
			_, _, err = repository.jobs.Enqueue(transactionCtx, queue.Job{
				Kind:        queue.KindGenerateSourceDocument,
				UniqueKey:   queue.StableJobHash(queue.KindGenerateSourceDocument, "recovery", command.RunSHA256, strconv.FormatInt(target.evidenceReferenceID, 10), target.inputHash),
				DurableArgs: args, ScheduledAt: repository.now().UTC(), MaxAttempts: 5, Priority: 3,
			})
			if err != nil {
				return fmt.Errorf("enqueue search rebuild: %w", err)
			}
		}
		after, err := repository.inspect(transactionCtx)
		if err != nil {
			return err
		}
		if before.inspection.Facts != after.inspection.Facts ||
			before.inspection.VaultManualRegionFingerprintSHA256 != after.inspection.VaultManualRegionFingerprintSHA256 ||
			after.inspection.DisposableDeliveryClaimCount != 0 ||
			after.inspection.StartedDeliveryClaimCount != before.inspection.StartedDeliveryClaimCount ||
			after.inspection.UnknownDeliveryAttemptCount != before.inspection.UnknownDeliveryAttemptCount {
			return fmt.Errorf("%w: recovery changed protected facts", sharedrepository.ErrConflict)
		}
		var runID int64
		if err := transaction.SQL.QueryRowContext(transactionCtx, `
INSERT INTO projection_recovery_runs(
  run_sha256,status,operator_record_id,reviewer_record_id,backup_evidence_sha256,rehearsal_evidence_sha256,
  notification_facts_before_sha256,notification_facts_after_sha256,
  vault_manual_before_sha256,vault_manual_after_sha256,
  notification_outbox_count,user_notification_count,read_receipt_count,delivery_attempt_count,
  max_user_notification_id,max_read_receipt_id,max_delivery_attempt_id,
  removed_delivery_claim_count,scheduled_vault_recovery_count,scheduled_search_rebuild_count,
  preserved_started_claim_count,preserved_unknown_attempt_count
) VALUES (
  $1,'scheduled',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21
) RETURNING id`,
			command.RunSHA256, command.OperatorID, command.ReviewerID,
			command.BackupEvidenceSHA256, command.RehearsalEvidenceSHA256,
			before.inspection.Facts.FingerprintSHA256, after.inspection.Facts.FingerprintSHA256,
			before.inspection.VaultManualRegionFingerprintSHA256, after.inspection.VaultManualRegionFingerprintSHA256,
			before.inspection.Facts.NotificationOutboxCount, before.inspection.Facts.UserNotificationCount,
			before.inspection.Facts.ReadReceiptCount, before.inspection.Facts.DeliveryAttemptCount,
			before.inspection.Facts.MaxUserNotificationID, before.inspection.Facts.MaxReadReceiptID,
			before.inspection.Facts.MaxDeliveryAttemptID, removed, len(before.vaultTargets), len(before.searchTargets),
			after.inspection.StartedDeliveryClaimCount, after.inspection.UnknownDeliveryAttemptCount,
		).Scan(&runID); err != nil {
			return databaserepository.MapError(err)
		}
		receipt = operationsapplication.ProjectionRecoveryReceiptDTO{
			RunID: runID, RunSHA256: command.RunSHA256, Status: "scheduled",
			BeforeFacts: before.inspection.Facts, AfterFacts: after.inspection.Facts,
			BeforeVaultManualRegionFingerprintSHA256: before.inspection.VaultManualRegionFingerprintSHA256,
			AfterVaultManualRegionFingerprintSHA256:  after.inspection.VaultManualRegionFingerprintSHA256,
			RemovedDeliveryClaimCount:                removed,
			ScheduledVaultRecoveryCount:              int64(len(before.vaultTargets)),
			ScheduledSearchRebuildCount:              int64(len(before.searchTargets)),
			PreservedStartedClaimCount:               after.inspection.StartedDeliveryClaimCount,
			PreservedUnknownAttemptCount:             after.inspection.UnknownDeliveryAttemptCount,
			Differences:                              []string{},
		}
		return nil
	})
	return receipt, err
}

func (repository *ProjectionRecoveryRepository) inspect(ctx context.Context) (projectionRecoveryCatalog, error) {
	if repository == nil || repository.runtime == nil || repository.vault == nil {
		return projectionRecoveryCatalog{}, sharedrepository.ErrUnavailable
	}
	queryer := projectionRecoveryQueryerFor(ctx, repository.runtime)
	facts, err := readProjectionRecoveryFacts(ctx, queryer)
	if err != nil {
		return projectionRecoveryCatalog{}, err
	}
	inspection := operationsapplication.ProjectionRecoveryInspectionDTO{Facts: facts, Blockers: []string{}}
	if err := queryer.QueryRowContext(ctx, `
SELECT count(*) FILTER (WHERE dispatch_started_at IS NULL),
       count(*) FILTER (WHERE dispatch_started_at IS NOT NULL)
FROM notification_delivery_claims`).Scan(&inspection.DisposableDeliveryClaimCount, &inspection.StartedDeliveryClaimCount); err != nil {
		return projectionRecoveryCatalog{}, databaserepository.MapError(err)
	}
	if err := queryer.QueryRowContext(ctx, `SELECT count(*) FROM notification_delivery_attempts WHERE status='unknown'`).Scan(&inspection.UnknownDeliveryAttemptCount); err != nil {
		return projectionRecoveryCatalog{}, databaserepository.MapError(err)
	}
	if inspection.StartedDeliveryClaimCount > 0 {
		inspection.Blockers = append(inspection.Blockers, "started_delivery_claim_requires_provider_reconciliation")
	}
	vaultTargets, manualFingerprint, vaultBlockers, err := repository.inspectVaultTargets(ctx, queryer)
	if err != nil {
		return projectionRecoveryCatalog{}, err
	}
	inspection.VaultManualRegionFingerprintSHA256 = manualFingerprint
	inspection.MissingVaultProjectionCount = int64(len(vaultTargets))
	inspection.Blockers = append(inspection.Blockers, vaultBlockers...)
	searchTargets, searchBlockers, err := inspectSearchTargets(ctx, queryer)
	if err != nil {
		return projectionRecoveryCatalog{}, err
	}
	inspection.MissingSearchProjectionCount = int64(len(searchTargets))
	inspection.Blockers = append(inspection.Blockers, searchBlockers...)
	return projectionRecoveryCatalog{inspection: inspection, vaultTargets: vaultTargets, searchTargets: searchTargets}, nil
}

func readProjectionRecoveryFacts(ctx context.Context, queryer projectionRecoveryQueryer) (operationsapplication.ProjectionRecoveryFactSnapshotDTO, error) {
	digest := sha256.New()
	tables := []string{"notification_outbox_events", "user_notifications", "notification_read_receipts", "notification_delivery_attempts"}
	for _, table := range tables {
		if err := hashProjectionRecoveryTable(ctx, queryer, digest, table); err != nil {
			return operationsapplication.ProjectionRecoveryFactSnapshotDTO{}, err
		}
	}
	var snapshot operationsapplication.ProjectionRecoveryFactSnapshotDTO
	err := queryer.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM notification_outbox_events),
  (SELECT count(*) FROM user_notifications),
  (SELECT count(*) FROM notification_read_receipts),
  (SELECT count(*) FROM notification_delivery_attempts),
  (SELECT coalesce(max(id),0) FROM user_notifications),
  (SELECT coalesce(max(id),0) FROM notification_read_receipts),
  (SELECT coalesce(max(id),0) FROM notification_delivery_attempts)`).Scan(
		&snapshot.NotificationOutboxCount, &snapshot.UserNotificationCount, &snapshot.ReadReceiptCount,
		&snapshot.DeliveryAttemptCount, &snapshot.MaxUserNotificationID, &snapshot.MaxReadReceiptID,
		&snapshot.MaxDeliveryAttemptID)
	if err != nil {
		return operationsapplication.ProjectionRecoveryFactSnapshotDTO{}, databaserepository.MapError(err)
	}
	snapshot.FingerprintSHA256 = hex.EncodeToString(digest.Sum(nil))
	return snapshot, nil
}

func hashProjectionRecoveryTable(ctx context.Context, queryer projectionRecoveryQueryer, digest hash.Hash, table string) error {
	allowed := map[string]bool{
		"notification_outbox_events": true, "user_notifications": true,
		"notification_read_receipts": true, "notification_delivery_attempts": true,
	}
	if !allowed[table] {
		return errors.New("invalid projection recovery fact table")
	}
	rows, err := queryer.QueryContext(ctx, `SELECT id,to_jsonb(fact)::text FROM `+table+` AS fact ORDER BY id`)
	if err != nil {
		return databaserepository.MapError(err)
	}
	defer rows.Close()
	writeRecoveryDigestPart(digest, table)
	for rows.Next() {
		var id int64
		var canonical string
		if err := rows.Scan(&id, &canonical); err != nil {
			return databaserepository.MapError(err)
		}
		writeRecoveryDigestPart(digest, strconv.FormatInt(id, 10))
		writeRecoveryDigestPart(digest, canonical)
	}
	return databaserepository.MapError(rows.Err())
}

func (repository *ProjectionRecoveryRepository) inspectVaultTargets(ctx context.Context, queryer projectionRecoveryQueryer) ([]vaultRecoveryTarget, string, []string, error) {
	rows, err := queryer.QueryContext(ctx, `
SELECT id,version,revision_no,btrim(content_hash)
FROM knowledge_documents
WHERE status='active' AND revision_no>0
ORDER BY id`)
	if err != nil {
		return nil, "", nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	targets := make([]vaultRecoveryTarget, 0)
	blockers := make([]string, 0)
	digest := sha256.New()
	writeRecoveryDigestPart(digest, "vault_manual_regions")
	for rows.Next() {
		var id, version, revision int64
		var contentHash string
		if err := rows.Scan(&id, &version, &revision, &contentHash); err != nil {
			return nil, "", nil, databaserepository.MapError(err)
		}
		inspection, err := repository.vault.Inspect(ctx, id)
		if err != nil || inspection.DocumentID != id || inspection.RevisionNo != revision || inspection.ContentHash != contentHash || len(inspection.HumanRegionSHA256) != 64 {
			blockers = append(blockers, "vault_projection_requires_manual_reconciliation")
			continue
		}
		writeRecoveryDigestPart(digest, strconv.FormatInt(id, 10))
		writeRecoveryDigestPart(digest, inspection.HumanRegionSHA256)
		if inspection.Missing {
			targets = append(targets, vaultRecoveryTarget{documentID: id, version: version, inputHash: contentHash})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", nil, databaserepository.MapError(err)
	}
	return targets, hex.EncodeToString(digest.Sum(nil)), blockers, nil
}

func inspectSearchTargets(ctx context.Context, queryer projectionRecoveryQueryer) ([]searchRebuildTarget, []string, error) {
	rows, err := queryer.QueryContext(ctx, `
SELECT version.id,version.version,btrim(version.content_sha256),
       min(evidence.id),count(evidence.id)
FROM documents AS document
JOIN document_versions AS version ON version.id=document.current_document_version_id
JOIN derived_artifacts AS artifact
  ON artifact.document_version_id=version.id AND artifact.artifact_type='plaintext'
 AND artifact.lifecycle_state='derived_available' AND artifact.active
JOIN source_observation_evidences AS evidence
  ON evidence.source_observation_id=version.source_observation_id AND evidence.usage='document_source'
WHERE document.document_state='active'
  AND version.lifecycle_state IN ('derived_available','readable')
  AND version.completeness<>'metadata_only'
  AND artifact.retention_until>CURRENT_TIMESTAMP
  AND current_rights_action_allowed(
      artifact.store_derived_rights_decision_id,artifact.source_connection_id,'document_version',
      version.id::text,version.content_sha256,'store_derived',CURRENT_TIMESTAMP)
  AND current_rights_action_allowed(
      artifact.retain_rights_decision_id,artifact.source_connection_id,'document_version',
      version.id::text,version.content_sha256,'retain',CURRENT_TIMESTAMP)
  AND NOT EXISTS (
      SELECT 1 FROM document_version_search_indexes AS search
      WHERE search.document_version_id=version.id AND search.lifecycle_state='active'
        AND search.retention_until>CURRENT_TIMESTAMP
  )
GROUP BY version.id,version.version,version.content_sha256
ORDER BY version.id`)
	if err != nil {
		return nil, nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	targets := make([]searchRebuildTarget, 0)
	blockers := make([]string, 0)
	for rows.Next() {
		var target searchRebuildTarget
		var evidenceCount int64
		var version int64
		if err := rows.Scan(&target.documentVersionID, &version, &target.inputHash, &target.evidenceReferenceID, &evidenceCount); err != nil {
			return nil, nil, databaserepository.MapError(err)
		}
		if evidenceCount != 1 {
			blockers = append(blockers, "search_projection_source_evidence_ambiguous")
			continue
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, databaserepository.MapError(err)
	}
	return targets, blockers, nil
}

func projectionRecoveryCatalogMatchesCommand(catalog projectionRecoveryCatalog, command operationsapplication.ApplyProjectionRecoveryCommand) bool {
	inspection := catalog.inspection
	return len(inspection.Blockers) == 0 && inspection.Facts == command.ExpectedFacts &&
		inspection.VaultManualRegionFingerprintSHA256 == command.ExpectedVaultManualRegionFingerprintSHA256 &&
		inspection.DisposableDeliveryClaimCount == command.ExpectedDisposableClaimCount &&
		inspection.StartedDeliveryClaimCount == command.ExpectedStartedClaimCount &&
		inspection.UnknownDeliveryAttemptCount == command.ExpectedUnknownAttemptCount &&
		inspection.MissingVaultProjectionCount == command.ExpectedVaultRecoveryCount &&
		inspection.MissingSearchProjectionCount == command.ExpectedSearchRebuildCount
}

func writeRecoveryDigestPart(digest hash.Hash, value string) {
	_, _ = fmt.Fprintf(digest, "%d:", len(value))
	_, _ = digest.Write([]byte(value))
}

type projectionRecoveryQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func projectionRecoveryQueryerFor(ctx context.Context, runtime *database.Runtime) projectionRecoveryQueryer {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return runtime.SQL
}
