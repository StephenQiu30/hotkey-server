package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const (
	rawEvidenceReconciliationCursorBase int64 = 1_000_000_000_000
	derivedReconciliationCursorBase     int64 = 2_000_000_000_000
	rawOrphanReconciliationCursorBase   int64 = 3_000_000_000_000
	vaultOrphanReconciliationCursorBase int64 = 4_000_000_000_000
	maximumReconciliationIdentity       int64 = 999_999_999_999
)

var _ operationsapplication.EvidenceLineageReconciliationRepository = (*EvidenceLineageMaintenanceRepository)(nil)

type evidenceLineageAssetManifestRecord struct {
	Cursor             int64
	AssetType          string
	AssetID            int64
	SourceConnectionID int64
	Locator            string
	ExpectedSHA256     string
	ExpectedSizeBytes  int64
	LifecycleState     string
	RetentionUntil     time.Time
	StoreAllowed       bool
	RetainAllowed      bool
	ExceptionApproved  bool
	Active             bool
	DocumentVersionID  *int64
	DocumentLifecycle  *string
	ObservedAt         *time.Time
}

type evidenceLineageFindingRecord struct {
	Manifest       evidenceLineageAssetManifestRecord
	Finding        string
	ObservedSHA256 *string
	LifecycleAfter *string
	RepairAction   *string
	ReasonCode     string
}

type evidenceLineageReconciliationRunRecord struct {
	ID               int64
	Version          int64
	Scope            string
	Status           string
	BatchSize        int
	GracePeriodHours int
	LastAssetCursor  int64
	ExaminedCount    int64
	HealthyCount     int64
	FindingCount     int64
	RepairedCount    int64
	FailedCount      int64
}

func (repository *EvidenceLineageMaintenanceRepository) InspectEvidenceLineageReconciliation(ctx context.Context, query operationsapplication.EvidenceLineageReconciliationInspectionQuery) (operationsapplication.EvidenceLineageReconciliationInspectionDTO, error) {
	if err := repository.validateReconciliationQuery(query.Scope, query.BatchSize, query.GracePeriodHours); err != nil {
		return operationsapplication.EvidenceLineageReconciliationInspectionDTO{}, err
	}
	verification, err := database.Verify(ctx, repository.runtime.Pool)
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationInspectionDTO{}, err
	}
	active, err := repository.activeEvidenceLineageProducerCount(ctx)
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationInspectionDTO{}, err
	}
	inspection := operationsapplication.EvidenceLineageReconciliationInspectionDTO{
		Scope: query.Scope, ActiveProducerCount: active, CatalogFingerprint: verification.CatalogFingerprint,
		Blockers: []string{}, FindingCounts: []operationsapplication.EvidenceLineageFindingCountDTO{},
	}
	if active > 0 {
		inspection.Blockers = append(inspection.Blockers, "active_evidence_lineage_producers")
	}
	if reconciliationUsesMinIO(query.Scope) && repository.rawObjects == nil {
		inspection.Blockers = append(inspection.Blockers, "minio_inspector_unavailable")
	}
	if reconciliationUsesVault(query.Scope) && repository.vaultFiles == nil {
		inspection.Blockers = append(inspection.Blockers, "vault_inspector_unavailable")
	}
	count, err := repository.countReconciliationCandidates(ctx, query.Scope)
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationInspectionDTO{}, err
	}
	inspection.CandidateCount = count
	if len(inspection.Blockers) == 0 {
		counts, scanErr := repository.inspectAllReconciliationCandidates(ctx, query)
		if scanErr != nil {
			return operationsapplication.EvidenceLineageReconciliationInspectionDTO{}, scanErr
		}
		findings := make([]string, 0, len(counts))
		for finding := range counts {
			findings = append(findings, finding)
		}
		sort.Strings(findings)
		inspection.CandidateCount = 0
		for _, finding := range findings {
			inspection.FindingCounts = append(inspection.FindingCounts, operationsapplication.EvidenceLineageFindingCountDTO{Finding: finding, Count: counts[finding]})
			inspection.CandidateCount += counts[finding]
		}
	}
	sort.Strings(inspection.Blockers)
	return inspection, nil
}

func (repository *EvidenceLineageMaintenanceRepository) StartEvidenceLineageReconciliation(ctx context.Context, command operationsapplication.StartEvidenceLineageReconciliationCommand) (operationsapplication.EvidenceLineageReconciliationRunDTO, error) {
	if err := validateStartEvidenceLineageReconciliationRecord(command); err != nil {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, err
	}
	transaction, err := repository.runtime.SQL.BeginTx(ctx, nil)
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, err
	}
	defer func() { _ = transaction.Rollback() }()
	if err := lockEvidenceLineageMaintenance(ctx, transaction); err != nil {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, err
	}
	active, err := activeEvidenceLineageProducerCountWithExecutor(ctx, transaction)
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, err
	}
	if active != 0 {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, fmt.Errorf("%w: evidence lineage producer is active", sharedrepository.ErrConflict)
	}
	row := transaction.QueryRowContext(ctx, `
INSERT INTO evidence_lineage_reconciliation_runs (
  scope,operator_id,reviewer_id,binary_sha256,schema_sha256,configuration_sha256,
  backup_evidence_sha256,rehearsal_evidence_sha256,batch_size,grace_period_hours
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id,version,scope,status,batch_size,grace_period_hours,last_asset_id,examined_count,
          healthy_count,finding_count,repaired_count,failed_count`,
		command.Scope, command.OperatorID, command.ReviewerID, command.BinarySHA256, command.SchemaSHA256,
		command.ConfigurationSHA256, command.BackupEvidenceSHA256, command.RehearsalEvidenceSHA256,
		command.BatchSize, command.GracePeriodHours)
	record, err := scanEvidenceLineageReconciliationRun(row)
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, err
	}
	if err := transaction.Commit(); err != nil {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, err
	}
	return evidenceLineageReconciliationRunDTO(record), nil
}

func (repository *EvidenceLineageMaintenanceRepository) ResumeEvidenceLineageReconciliation(ctx context.Context, command operationsapplication.ResumeEvidenceLineageReconciliationCommand) (operationsapplication.EvidenceLineageReconciliationRunDTO, error) {
	if command.RunID <= 0 {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, fmt.Errorf("%w: invalid reconciliation run", sharedrepository.ErrInvalidInput)
	}
	row := repository.runtime.SQL.QueryRowContext(ctx, `
UPDATE evidence_lineage_reconciliation_runs
SET version=version+1,resume_count=resume_count+1,updated_at=now()
WHERE id=$1 AND status='running' AND scope=$2 AND batch_size=$3 AND grace_period_hours=$4
  AND operator_id=$5 AND reviewer_id=$6 AND binary_sha256=$7 AND schema_sha256=$8
  AND configuration_sha256=$9 AND backup_evidence_sha256=$10 AND rehearsal_evidence_sha256=$11
RETURNING id,version,scope,status,batch_size,grace_period_hours,last_asset_id,examined_count,
          healthy_count,finding_count,repaired_count,failed_count`,
		command.RunID, command.Scope, command.BatchSize, command.GracePeriodHours,
		command.OperatorID, command.ReviewerID, command.BinarySHA256, command.SchemaSHA256,
		command.ConfigurationSHA256, command.BackupEvidenceSHA256, command.RehearsalEvidenceSHA256)
	record, err := scanEvidenceLineageReconciliationRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, fmt.Errorf("%w: reconciliation resume facts changed", sharedrepository.ErrConflict)
	}
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, err
	}
	return evidenceLineageReconciliationRunDTO(record), nil
}

func (repository *EvidenceLineageMaintenanceRepository) ApplyEvidenceLineageReconciliationBatch(ctx context.Context, command operationsapplication.EvidenceLineageReconciliationBatchCommand) (operationsapplication.EvidenceLineageReconciliationBatchResultDTO, error) {
	if command.RunID <= 0 || command.AfterAssetCursor < 0 || repository.validateReconciliationQuery(command.Scope, command.BatchSize, command.GracePeriodHours) != nil {
		return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, fmt.Errorf("%w: invalid reconciliation batch", sharedrepository.ErrInvalidInput)
	}
	transaction, err := repository.runtime.SQL.BeginTx(ctx, nil)
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, err
	}
	defer func() { _ = transaction.Rollback() }()
	var scope, status string
	var cursor int64
	if err := transaction.QueryRowContext(ctx, `SELECT scope,status,last_asset_id FROM evidence_lineage_reconciliation_runs WHERE id=$1 FOR UPDATE`, command.RunID).Scan(&scope, &status, &cursor); err != nil {
		return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, err
	}
	if scope != command.Scope || status != "running" || cursor != command.AfterAssetCursor {
		return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, fmt.Errorf("%w: reconciliation cursor changed", sharedrepository.ErrConflict)
	}
	manifests, hasMore, err := repository.queryEvidenceLineageAssetManifests(ctx, transaction, command.Scope, cursor, command.BatchSize)
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, err
	}
	result := operationsapplication.EvidenceLineageReconciliationBatchResultDTO{RunID: command.RunID, LastAssetCursor: cursor, HasMore: hasMore}
	for _, manifest := range manifests {
		finding, inspectErr := repository.inspectEvidenceLineageManifest(ctx, command.Scope, command.GracePeriodHours, manifest)
		if inspectErr != nil {
			return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, inspectErr
		}
		if err := applyEvidenceLineageFinding(ctx, transaction, &finding); err != nil {
			return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, err
		}
		if err := insertEvidenceLineageFinding(ctx, transaction, command.RunID, command.Scope, finding); err != nil {
			return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, err
		}
		result.ExaminedCount++
		result.LastAssetCursor = manifest.Cursor
		if finding.Finding == string(operationsdomain.ReconciliationFindingHealthy) {
			result.HealthyCount++
		} else {
			result.FindingCount++
			if finding.RepairAction != nil && *finding.RepairAction != "none" {
				result.RepairedCount++
			}
		}
	}
	if _, err := transaction.ExecContext(ctx, `
UPDATE evidence_lineage_reconciliation_runs
SET version=version+1,last_asset_id=$2,examined_count=examined_count+$3,
    healthy_count=healthy_count+$4,finding_count=finding_count+$5,repaired_count=repaired_count+$6,
    failed_count=failed_count+$7,updated_at=now()
WHERE id=$1`, command.RunID, result.LastAssetCursor, result.ExaminedCount, result.HealthyCount,
		result.FindingCount, result.RepairedCount, result.FailedCount); err != nil {
		return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, err
	}
	if err := transaction.Commit(); err != nil {
		return operationsapplication.EvidenceLineageReconciliationBatchResultDTO{}, err
	}
	return result, nil
}

func (repository *EvidenceLineageMaintenanceRepository) CompleteEvidenceLineageReconciliation(ctx context.Context, command operationsapplication.CompleteEvidenceLineageReconciliationCommand) (operationsapplication.EvidenceLineageReconciliationRunDTO, error) {
	row := repository.runtime.SQL.QueryRowContext(ctx, `
UPDATE evidence_lineage_reconciliation_runs
SET version=version+1,status='completed',completed_at=now(),updated_at=now()
WHERE id=$1 AND status='running' AND last_asset_id=$2
RETURNING id,version,scope,status,batch_size,grace_period_hours,last_asset_id,examined_count,
          healthy_count,finding_count,repaired_count,failed_count`, command.RunID, command.LastAssetCursor)
	record, err := scanEvidenceLineageReconciliationRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, fmt.Errorf("%w: reconciliation completion changed", sharedrepository.ErrConflict)
	}
	if err != nil {
		return operationsapplication.EvidenceLineageReconciliationRunDTO{}, err
	}
	return evidenceLineageReconciliationRunDTO(record), nil
}

func (repository *EvidenceLineageMaintenanceRepository) validateReconciliationQuery(scope string, batchSize, graceHours int) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || repository.runtime.Pool == nil ||
		!operationsdomain.EvidenceLineageReconciliationScope(scope).Valid() || batchSize < 1 || batchSize > 1000 || graceHours < 1 || graceHours > 720 {
		return fmt.Errorf("%w: invalid evidence lineage reconciliation query", sharedrepository.ErrInvalidInput)
	}
	return nil
}

func (repository *EvidenceLineageMaintenanceRepository) countReconciliationCandidates(ctx context.Context, scope string) (int64, error) {
	var raw, derived int64
	if reconciliationUsesMinIO(scope) || scope == string(operationsdomain.ReconciliationScopeRightsRetention) {
		if err := repository.runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM evidence_snapshots`).Scan(&raw); err != nil {
			return 0, err
		}
	}
	if reconciliationUsesVault(scope) || scope == string(operationsdomain.ReconciliationScopeRightsRetention) {
		if err := repository.runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM derived_artifacts`).Scan(&derived); err != nil {
			return 0, err
		}
	}
	return raw + derived, nil
}

func (repository *EvidenceLineageMaintenanceRepository) inspectAllReconciliationCandidates(ctx context.Context, query operationsapplication.EvidenceLineageReconciliationInspectionQuery) (map[string]int64, error) {
	counts := make(map[string]int64)
	after := int64(0)
	for {
		transaction, err := repository.runtime.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		manifests, hasMore, queryErr := repository.queryEvidenceLineageAssetManifests(ctx, transaction, query.Scope, after, query.BatchSize)
		if queryErr != nil {
			_ = transaction.Rollback()
			return nil, queryErr
		}
		for _, manifest := range manifests {
			finding, inspectErr := repository.inspectEvidenceLineageManifest(ctx, query.Scope, query.GracePeriodHours, manifest)
			if inspectErr != nil {
				_ = transaction.Rollback()
				return nil, inspectErr
			}
			counts[finding.Finding]++
			after = manifest.Cursor
		}
		if err := transaction.Commit(); err != nil {
			return nil, err
		}
		if !hasMore {
			return counts, nil
		}
	}
}

func (repository *EvidenceLineageMaintenanceRepository) inspectEvidenceLineageManifest(ctx context.Context, scope string, gracePeriodHours int, manifest evidenceLineageAssetManifestRecord) (evidenceLineageFindingRecord, error) {
	finding := evidenceLineageFindingRecord{Manifest: manifest, Finding: string(operationsdomain.ReconciliationFindingHealthy), ReasonCode: "asset_verified"}
	if manifest.AssetType == "raw_object_orphan" || manifest.AssetType == "vault_file_orphan" {
		if manifest.ObservedAt == nil {
			return finding, errors.New("orphan asset modification time is unavailable")
		}
		noAction := "none"
		finding.RepairAction = &noAction
		if manifest.ObservedAt.After(time.Now().UTC().Add(-time.Duration(gracePeriodHours) * time.Hour)) {
			finding.Finding, finding.ReasonCode = string(operationsdomain.ReconciliationFindingOrphanWithinGrace), "untracked_asset_within_grace"
			return finding, nil
		}
		finding.Finding, finding.ReasonCode = string(operationsdomain.ReconciliationFindingOrphanExpired), "untracked_asset_grace_expired"
		return finding, nil
	}
	if manifest.AssetType == "evidence_snapshot" && reconciliationUsesMinIO(scope) {
		inspection, err := repository.rawObjects.InspectRawEvidenceObject(ctx, manifest.Locator, maximumReconciliationAssetBytes)
		if err != nil {
			return finding, err
		}
		if !inspection.Exists {
			finding.Finding, finding.ReasonCode = string(operationsdomain.ReconciliationFindingMissing), "raw_object_missing"
			return withFindingRepair(finding, "quarantined", "quarantine_asset"), nil
		}
		finding.ObservedSHA256 = &inspection.SHA256
		if inspection.SHA256 != manifest.ExpectedSHA256 || inspection.SizeBytes != manifest.ExpectedSizeBytes {
			finding.Finding, finding.ReasonCode = string(operationsdomain.ReconciliationFindingDigestMismatch), "raw_object_integrity_mismatch"
			return withFindingRepair(finding, "quarantined", "quarantine_asset"), nil
		}
	}
	if manifest.AssetType == "derived_artifact" && reconciliationUsesVault(scope) {
		inspection, err := repository.vaultFiles.InspectVaultProjection(ctx, manifest.Locator, maximumReconciliationAssetBytes)
		if err != nil {
			return finding, err
		}
		if !inspection.Exists {
			finding.Finding, finding.ReasonCode = string(operationsdomain.ReconciliationFindingMissing), "vault_projection_missing"
			return withFindingRepair(finding, "quarantined", "quarantine_asset"), nil
		}
		finding.ObservedSHA256 = &inspection.SHA256
		if inspection.SHA256 != manifest.ExpectedSHA256 || inspection.SizeBytes != manifest.ExpectedSizeBytes {
			finding.Finding, finding.ReasonCode = string(operationsdomain.ReconciliationFindingDigestMismatch), "vault_projection_integrity_mismatch"
			return withFindingRepair(finding, "quarantined", "quarantine_asset"), nil
		}
	}
	if reconciliationUsesRights(scope) {
		if !manifest.StoreAllowed {
			finding.Finding, finding.ReasonCode = string(operationsdomain.ReconciliationFindingPolicyBlocked), "current_storage_right_denied"
			if manifest.AssetType == "evidence_snapshot" {
				return withFindingRepair(finding, "policy_blocked", "block_asset"), nil
			}
			return withFindingRepair(finding, "retention_blocked", "block_retention"), nil
		}
		retentionExpired := !manifest.RetentionUntil.After(time.Now().UTC())
		if !manifest.RetainAllowed || retentionExpired && !manifest.ExceptionApproved {
			finding.Finding, finding.ReasonCode = string(operationsdomain.ReconciliationFindingRetentionBlocked), "current_retention_right_denied_or_expired"
			return withFindingRepair(finding, "retention_blocked", "block_retention"), nil
		}
		if retentionExpired && manifest.ExceptionApproved {
			finding.ReasonCode = "approved_retention_exception"
		}
	}
	if manifest.Active && manifest.DocumentLifecycle != nil && *manifest.DocumentLifecycle == "readable" && manifest.LifecycleState != "derived_available" {
		finding.Finding, finding.ReasonCode = string(operationsdomain.ReconciliationFindingActivePointerInvalid), "readable_document_active_artifact_invalid"
		return withFindingRepair(finding, "quarantined", "quarantine_asset"), nil
	}
	return finding, nil
}

func withFindingRepair(finding evidenceLineageFindingRecord, lifecycle, action string) evidenceLineageFindingRecord {
	finding.LifecycleAfter = &lifecycle
	finding.RepairAction = &action
	return finding
}

func applyEvidenceLineageFinding(ctx context.Context, transaction *sql.Tx, finding *evidenceLineageFindingRecord) error {
	if finding == nil || finding.Finding == string(operationsdomain.ReconciliationFindingHealthy) || finding.LifecycleAfter == nil {
		return nil
	}
	manifest := finding.Manifest
	if manifest.LifecycleState == *finding.LifecycleAfter {
		noAction := "none"
		finding.RepairAction = &noAction
		return nil
	}
	if manifest.AssetType == "evidence_snapshot" {
		result, err := transaction.ExecContext(ctx, `
UPDATE evidence_snapshots
SET lifecycle_state=$2,available_at=NULL,failure_code=NULL,updated_at=now()
WHERE id=$1 AND lifecycle_state=$3`, manifest.AssetID, *finding.LifecycleAfter, manifest.LifecycleState)
		if err != nil {
			return err
		}
		return requireOneMaintenanceMutation(result)
	}
	if manifest.AssetType == "derived_artifact" {
		result, err := transaction.ExecContext(ctx, `
UPDATE derived_artifacts
SET lifecycle_state=$2,active=false,available_at=NULL,failure_code=NULL,updated_at=now()
WHERE id=$1 AND lifecycle_state=$3`, manifest.AssetID, *finding.LifecycleAfter, manifest.LifecycleState)
		if err != nil {
			return err
		}
		if err := requireOneMaintenanceMutation(result); err != nil {
			return err
		}
		if manifest.DocumentVersionID != nil && manifest.DocumentLifecycle != nil && *manifest.DocumentLifecycle == "readable" {
			documentState := *finding.LifecycleAfter
			if documentState != "retention_blocked" {
				documentState = "quarantined"
			}
			if _, err := transaction.ExecContext(ctx, `UPDATE document_versions SET version=version+1,lifecycle_state=$2 WHERE id=$1 AND lifecycle_state='readable'`, *manifest.DocumentVersionID, documentState); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireOneMaintenanceMutation(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("%w: evidence lineage asset changed during reconciliation", sharedrepository.ErrConflict)
	}
	return nil
}

func insertEvidenceLineageFinding(ctx context.Context, transaction *sql.Tx, runID int64, scope string, finding evidenceLineageFindingRecord) error {
	manifest := finding.Manifest
	assetKeySHA := evidenceLineageAssetKeySHA256(manifest.AssetType, manifest.AssetID, manifest.Locator)
	_, err := transaction.ExecContext(ctx, `
INSERT INTO evidence_lineage_reconciliation_items (
  run_id,scope,asset_type,asset_id,asset_key_sha256,source_connection_id,finding,
  expected_sha256,observed_sha256,lifecycle_before,lifecycle_after,repair_action,reason_code
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		runID, scope, manifest.AssetType, nullableMaintenanceIdentity(manifest.AssetID), assetKeySHA, nullableMaintenanceIdentity(manifest.SourceConnectionID),
		finding.Finding, nullableMaintenanceString(manifest.ExpectedSHA256), finding.ObservedSHA256,
		manifest.LifecycleState, finding.LifecycleAfter, finding.RepairAction, finding.ReasonCode)
	return err
}

func nullableMaintenanceIdentity(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func evidenceLineageAssetKeySHA256(assetType string, assetID int64, locator string) string {
	digest := sha256.Sum256([]byte(assetType + "\x00" + strconv.FormatInt(assetID, 10) + "\x00" + locator))
	return hex.EncodeToString(digest[:])
}

func nullableMaintenanceString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (repository *EvidenceLineageMaintenanceRepository) queryEvidenceLineageAssetManifests(ctx context.Context, transaction *sql.Tx, scope string, after int64, batchSize int) ([]evidenceLineageAssetManifestRecord, bool, error) {
	records, persistedHasMore, err := queryPersistedEvidenceLineageAssetManifests(ctx, transaction, scope, after, batchSize)
	if err != nil || persistedHasMore {
		return records, persistedHasMore, err
	}
	limit := batchSize + 1
	if reconciliationUsesMinIO(scope) && after < vaultOrphanReconciliationCursorBase && len(records) < limit {
		orphans, listErr := repository.rawEvidenceOrphanManifests(ctx, transaction, after, limit-len(records))
		if listErr != nil {
			return nil, false, listErr
		}
		records = append(records, orphans...)
	}
	if reconciliationUsesVault(scope) && len(records) < limit {
		orphans, listErr := repository.vaultProjectionOrphanManifests(ctx, transaction, after, limit-len(records))
		if listErr != nil {
			return nil, false, listErr
		}
		records = append(records, orphans...)
	}
	hasMore := len(records) > batchSize
	if hasMore {
		records = records[:batchSize]
	}
	return records, hasMore, nil
}

func (repository *EvidenceLineageMaintenanceRepository) rawEvidenceOrphanManifests(ctx context.Context, transaction *sql.Tx, after int64, limit int) ([]evidenceLineageAssetManifestRecord, error) {
	if repository.rawObjects == nil || limit <= 0 || after >= vaultOrphanReconciliationCursorBase {
		return []evidenceLineageAssetManifestRecord{}, nil
	}
	objects, err := repository.rawObjects.ListRawEvidenceObjects(ctx, maximumReconciliationListedAssets)
	if err != nil {
		return nil, err
	}
	known, err := knownEvidenceLineageLocators(ctx, transaction, `SELECT object_key FROM evidence_snapshots`)
	if err != nil {
		return nil, err
	}
	return orphanEvidenceLineageManifests(objects, known, "raw_object_orphan", rawOrphanReconciliationCursorBase, after, limit)
}

func (repository *EvidenceLineageMaintenanceRepository) vaultProjectionOrphanManifests(ctx context.Context, transaction *sql.Tx, after int64, limit int) ([]evidenceLineageAssetManifestRecord, error) {
	if repository.vaultFiles == nil || limit <= 0 {
		return []evidenceLineageAssetManifestRecord{}, nil
	}
	files, err := repository.vaultFiles.ListVaultProjections(ctx, maximumReconciliationListedAssets)
	if err != nil {
		return nil, err
	}
	known, err := knownEvidenceLineageLocators(ctx, transaction, `SELECT vault_relative_path FROM derived_artifacts`)
	if err != nil {
		return nil, err
	}
	return orphanEvidenceLineageManifests(files, known, "vault_file_orphan", vaultOrphanReconciliationCursorBase, after, limit)
}

func knownEvidenceLineageLocators(ctx context.Context, transaction *sql.Tx, query string) (map[string]struct{}, error) {
	rows, err := transaction.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := make(map[string]struct{})
	for rows.Next() {
		var locator string
		if err := rows.Scan(&locator); err != nil {
			return nil, err
		}
		known[locator] = struct{}{}
	}
	return known, rows.Err()
}

func orphanEvidenceLineageManifests(objects []evidenceLineageStoredAssetRecord, known map[string]struct{}, assetType string, cursorBase, after int64, limit int) ([]evidenceLineageAssetManifestRecord, error) {
	sort.Slice(objects, func(left, right int) bool { return objects[left].Locator < objects[right].Locator })
	orphans := make([]evidenceLineageStoredAssetRecord, 0, len(objects))
	for _, object := range objects {
		if object.Locator == "" || object.ModifiedAt.IsZero() {
			return nil, errors.New("storage listing returned an invalid reconciliation asset")
		}
		if _, found := known[object.Locator]; !found {
			orphans = append(orphans, object)
		}
	}
	records := make([]evidenceLineageAssetManifestRecord, 0, limit)
	for index, orphan := range orphans {
		ordinal := int64(index + 1)
		if ordinal > maximumReconciliationIdentity {
			return nil, errors.New("orphan asset count exceeds maintenance cursor contract")
		}
		cursor := cursorBase + ordinal
		if cursor <= after {
			continue
		}
		observedAt := orphan.ModifiedAt.UTC()
		records = append(records, evidenceLineageAssetManifestRecord{
			Cursor: cursor, AssetType: assetType, Locator: orphan.Locator,
			LifecycleState: "untracked", ObservedAt: &observedAt,
		})
		if len(records) == limit {
			break
		}
	}
	return records, nil
}

func queryPersistedEvidenceLineageAssetManifests(ctx context.Context, transaction *sql.Tx, scope string, after int64, batchSize int) ([]evidenceLineageAssetManifestRecord, bool, error) {
	records := make([]evidenceLineageAssetManifestRecord, 0, batchSize+1)
	if (reconciliationUsesMinIO(scope) || reconciliationUsesRights(scope)) && after < derivedReconciliationCursorBase && len(records) < batchSize+1 {
		rawAfter := int64(0)
		if after >= rawEvidenceReconciliationCursorBase {
			rawAfter = after - rawEvidenceReconciliationCursorBase
		}
		rows, err := transaction.QueryContext(ctx, `
SELECT id,source_connection_id,object_key,payload_sha256,size_bytes,lifecycle_state,retention_until,
       current_rights_action_is_allowed(source_connection_id,'raw_response',btrim(snapshot_key),payload_sha256,'store_raw',CURRENT_TIMESTAMP),
       current_rights_retention_days(source_connection_id,'raw_response',btrim(snapshot_key),payload_sha256,CURRENT_TIMESTAMP) IS NOT NULL,
       EXISTS (
         SELECT 1 FROM evidence_retention_exceptions AS exception
         WHERE exception.evidence_snapshot_id=evidence_snapshots.id
           AND exception.revoked_at IS NULL
           AND exception.approved_at <= CURRENT_TIMESTAMP
           AND (exception.expires_at IS NULL OR exception.expires_at > CURRENT_TIMESTAMP)
       )
FROM evidence_snapshots WHERE id>$1 ORDER BY id LIMIT $2`, rawAfter, batchSize+1-len(records))
		if err != nil {
			return nil, false, err
		}
		for rows.Next() {
			var record evidenceLineageAssetManifestRecord
			if err := rows.Scan(&record.AssetID, &record.SourceConnectionID, &record.Locator, &record.ExpectedSHA256,
				&record.ExpectedSizeBytes, &record.LifecycleState, &record.RetentionUntil,
				&record.StoreAllowed, &record.RetainAllowed, &record.ExceptionApproved); err != nil {
				rows.Close()
				return nil, false, err
			}
			if record.AssetID > maximumReconciliationIdentity {
				rows.Close()
				return nil, false, errors.New("evidence snapshot ID exceeds maintenance cursor contract")
			}
			record.Cursor = rawEvidenceReconciliationCursorBase + record.AssetID
			record.AssetType = "evidence_snapshot"
			records = append(records, record)
		}
		if err := rows.Close(); err != nil {
			return nil, false, err
		}
	}
	if (reconciliationUsesVault(scope) || reconciliationUsesRights(scope)) && len(records) < batchSize+1 {
		derivedAfter := int64(0)
		if after >= derivedReconciliationCursorBase {
			derivedAfter = after - derivedReconciliationCursorBase
		}
		rows, err := transaction.QueryContext(ctx, `
SELECT artifact.id,artifact.source_connection_id,artifact.vault_relative_path,artifact.sha256,artifact.size_bytes,
       artifact.lifecycle_state,artifact.retention_until,
       current_rights_action_is_allowed(artifact.source_connection_id,'document_version',artifact.document_version_id::text,version.content_sha256,'store_derived',CURRENT_TIMESTAMP),
       current_rights_retention_days(artifact.source_connection_id,'document_version',artifact.document_version_id::text,version.content_sha256,CURRENT_TIMESTAMP) IS NOT NULL,
       false,
       artifact.active,artifact.document_version_id,version.lifecycle_state
FROM derived_artifacts artifact
JOIN document_versions version ON version.id=artifact.document_version_id
WHERE artifact.id>$1 ORDER BY artifact.id LIMIT $2`, derivedAfter, batchSize+1-len(records))
		if err != nil {
			return nil, false, err
		}
		for rows.Next() {
			var record evidenceLineageAssetManifestRecord
			var documentID int64
			var documentState string
			if err := rows.Scan(&record.AssetID, &record.SourceConnectionID, &record.Locator, &record.ExpectedSHA256,
				&record.ExpectedSizeBytes, &record.LifecycleState, &record.RetentionUntil,
				&record.StoreAllowed, &record.RetainAllowed, &record.ExceptionApproved, &record.Active, &documentID, &documentState); err != nil {
				rows.Close()
				return nil, false, err
			}
			if record.AssetID > maximumReconciliationIdentity {
				rows.Close()
				return nil, false, errors.New("derived artifact ID exceeds maintenance cursor contract")
			}
			record.Cursor = derivedReconciliationCursorBase + record.AssetID
			record.AssetType = "derived_artifact"
			record.DocumentVersionID = &documentID
			record.DocumentLifecycle = &documentState
			records = append(records, record)
		}
		if err := rows.Close(); err != nil {
			return nil, false, err
		}
	}
	hasMore := len(records) > batchSize
	if hasMore {
		records = records[:batchSize]
	}
	return records, hasMore, nil
}

func validateStartEvidenceLineageReconciliationRecord(command operationsapplication.StartEvidenceLineageReconciliationCommand) error {
	if !operationsdomain.EvidenceLineageReconciliationScope(command.Scope).Valid() || command.BatchSize < 1 || command.BatchSize > 1000 ||
		command.GracePeriodHours < 1 || command.GracePeriodHours > 720 || command.OperatorID == "" || command.OperatorID == command.ReviewerID || command.ReviewerID == "" {
		return fmt.Errorf("%w: invalid evidence lineage reconciliation run", sharedrepository.ErrInvalidInput)
	}
	for _, digest := range []string{command.BinarySHA256, command.SchemaSHA256, command.ConfigurationSHA256, command.BackupEvidenceSHA256, command.RehearsalEvidenceSHA256} {
		if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
			return fmt.Errorf("%w: invalid evidence lineage reconciliation digest", sharedrepository.ErrInvalidInput)
		}
	}
	return nil
}

func scanEvidenceLineageReconciliationRun(row interface{ Scan(...any) error }) (evidenceLineageReconciliationRunRecord, error) {
	var record evidenceLineageReconciliationRunRecord
	err := row.Scan(&record.ID, &record.Version, &record.Scope, &record.Status, &record.BatchSize,
		&record.GracePeriodHours, &record.LastAssetCursor, &record.ExaminedCount, &record.HealthyCount,
		&record.FindingCount, &record.RepairedCount, &record.FailedCount)
	return record, err
}

func evidenceLineageReconciliationRunDTO(record evidenceLineageReconciliationRunRecord) operationsapplication.EvidenceLineageReconciliationRunDTO {
	return operationsapplication.EvidenceLineageReconciliationRunDTO{
		RunID: record.ID, Status: record.Status, LastAssetCursor: record.LastAssetCursor,
		ExaminedCount: record.ExaminedCount, HealthyCount: record.HealthyCount,
		FindingCount: record.FindingCount, RepairedCount: record.RepairedCount, FailedCount: record.FailedCount,
	}
}

func reconciliationUsesMinIO(scope string) bool {
	return scope == string(operationsdomain.ReconciliationScopePostgresMinIO) || scope == string(operationsdomain.ReconciliationScopeAll)
}

func reconciliationUsesVault(scope string) bool {
	return scope == string(operationsdomain.ReconciliationScopePostgresVault) || scope == string(operationsdomain.ReconciliationScopeAll)
}

func reconciliationUsesRights(scope string) bool {
	return scope == string(operationsdomain.ReconciliationScopeRightsRetention) || scope == string(operationsdomain.ReconciliationScopeAll)
}
