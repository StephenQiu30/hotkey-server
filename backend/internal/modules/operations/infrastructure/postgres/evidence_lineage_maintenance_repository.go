package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var _ operationsapplication.EvidenceLineageMaintenanceRepository = (*EvidenceLineageMaintenanceRepository)(nil)

type EvidenceLineageMaintenanceRepository struct {
	runtime    *database.Runtime
	rawObjects rawEvidenceAssetInspector
	vaultFiles vaultProjectionAssetInspector
}

func NewEvidenceLineageMaintenanceRepository(runtime *database.Runtime) *EvidenceLineageMaintenanceRepository {
	return &EvidenceLineageMaintenanceRepository{runtime: runtime}
}

func newEvidenceLineageMaintenanceRepository(
	runtime *database.Runtime,
	rawObjects rawEvidenceAssetInspector,
	vaultFiles vaultProjectionAssetInspector,
) *EvidenceLineageMaintenanceRepository {
	return &EvidenceLineageMaintenanceRepository{runtime: runtime, rawObjects: rawObjects, vaultFiles: vaultFiles}
}

func (repository *EvidenceLineageMaintenanceRepository) InspectEvidenceLineageBackfill(ctx context.Context, query operationsapplication.EvidenceLineageBackfillInspectionQuery) (operationsapplication.EvidenceLineageBackfillInspectionDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || repository.runtime.Pool == nil ||
		!operationsdomain.EvidenceLineageMigrationPhase(query.Phase).Valid() || query.BatchSize < 1 || query.BatchSize > 1000 {
		return operationsapplication.EvidenceLineageBackfillInspectionDTO{}, fmt.Errorf("%w: invalid evidence lineage backfill inspection", sharedrepository.ErrInvalidInput)
	}
	verification, err := database.Verify(ctx, repository.runtime.Pool)
	if err != nil {
		return operationsapplication.EvidenceLineageBackfillInspectionDTO{}, fmt.Errorf("verify canonical catalog before evidence lineage backfill: %w", err)
	}
	candidateCount, err := repository.countBackfillCandidates(ctx, query.Phase)
	if err != nil {
		return operationsapplication.EvidenceLineageBackfillInspectionDTO{}, err
	}
	activeProducers, err := repository.activeEvidenceLineageProducerCount(ctx)
	if err != nil {
		return operationsapplication.EvidenceLineageBackfillInspectionDTO{}, err
	}
	inspection := operationsapplication.EvidenceLineageBackfillInspectionDTO{
		Phase: query.Phase, CandidateCount: candidateCount, ActiveProducerCount: activeProducers,
		CatalogFingerprint: verification.CatalogFingerprint, Blockers: []string{},
	}
	if activeProducers > 0 {
		inspection.Blockers = append(inspection.Blockers, "active_evidence_lineage_producers")
	}
	sort.Strings(inspection.Blockers)
	return inspection, nil
}

func (repository *EvidenceLineageMaintenanceRepository) StartEvidenceLineageBackfill(ctx context.Context, command operationsapplication.StartEvidenceLineageBackfillCommand) (operationsapplication.EvidenceLineageMaintenanceRunDTO, error) {
	if err := validateStartEvidenceLineageBackfillRecord(command); err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	transaction, err := repository.runtime.SQL.BeginTx(ctx, nil)
	if err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	defer func() { _ = transaction.Rollback() }()
	if err := lockEvidenceLineageMaintenance(ctx, transaction); err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	active, err := activeEvidenceLineageProducerCountWithExecutor(ctx, transaction)
	if err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	if active != 0 {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, fmt.Errorf("%w: evidence lineage producer is active", sharedrepository.ErrConflict)
	}
	row := transaction.QueryRowContext(ctx, `
INSERT INTO evidence_lineage_migration_runs (
  phase,operator_id,reviewer_id,binary_sha256,schema_sha256,configuration_sha256,
  backup_evidence_sha256,rehearsal_evidence_sha256,batch_size
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id,version,phase,status,batch_size,last_legacy_resource_id,examined_count,reused_count,
          created_count,skipped_count,blocked_count,failed_count,started_at,completed_at`,
		command.Phase, command.OperatorID, command.ReviewerID, command.BinarySHA256, command.SchemaSHA256,
		command.ConfigurationSHA256, command.BackupEvidenceSHA256, command.RehearsalEvidenceSHA256, command.BatchSize)
	record, err := scanEvidenceLineageMigrationRun(row)
	if err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, fmt.Errorf("insert evidence lineage migration run: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	return evidenceLineageMaintenanceRunDTOFromRecord(record), nil
}

func (repository *EvidenceLineageMaintenanceRepository) ResumeEvidenceLineageBackfill(ctx context.Context, command operationsapplication.ResumeEvidenceLineageBackfillCommand) (operationsapplication.EvidenceLineageMaintenanceRunDTO, error) {
	if command.RunID <= 0 {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, fmt.Errorf("%w: invalid evidence lineage migration run", sharedrepository.ErrInvalidInput)
	}
	transaction, err := repository.runtime.SQL.BeginTx(ctx, nil)
	if err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	defer func() { _ = transaction.Rollback() }()
	if err := lockEvidenceLineageMaintenance(ctx, transaction); err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	row := transaction.QueryRowContext(ctx, `
UPDATE evidence_lineage_migration_runs
SET version=version+1,resume_count=resume_count+1,updated_at=now()
WHERE id=$1 AND status='running' AND phase=$2 AND batch_size=$3 AND operator_id=$4 AND reviewer_id=$5
  AND binary_sha256=$6 AND schema_sha256=$7 AND configuration_sha256=$8
  AND backup_evidence_sha256=$9 AND rehearsal_evidence_sha256=$10
RETURNING id,version,phase,status,batch_size,last_legacy_resource_id,examined_count,reused_count,
          created_count,skipped_count,blocked_count,failed_count,started_at,completed_at`,
		command.RunID, command.Phase, command.BatchSize, command.OperatorID, command.ReviewerID,
		command.BinarySHA256, command.SchemaSHA256, command.ConfigurationSHA256,
		command.BackupEvidenceSHA256, command.RehearsalEvidenceSHA256)
	record, err := scanEvidenceLineageMigrationRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, fmt.Errorf("%w: evidence lineage migration resume facts changed", sharedrepository.ErrConflict)
	}
	if err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	if err := transaction.Commit(); err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	return evidenceLineageMaintenanceRunDTOFromRecord(record), nil
}

func (repository *EvidenceLineageMaintenanceRepository) ApplyEvidenceLineageBackfillBatch(ctx context.Context, command operationsapplication.EvidenceLineageBackfillBatchCommand) (operationsapplication.EvidenceLineageBackfillBatchResultDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || command.RunID <= 0 || command.AfterResourceID < 0 ||
		!operationsdomain.EvidenceLineageMigrationPhase(command.Phase).Valid() || command.BatchSize < 1 || command.BatchSize > 1000 {
		return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, fmt.Errorf("%w: invalid evidence lineage backfill batch", sharedrepository.ErrInvalidInput)
	}
	transaction, err := repository.runtime.SQL.BeginTx(ctx, nil)
	if err != nil {
		return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, err
	}
	defer func() { _ = transaction.Rollback() }()
	var phase, status string
	var cursor int64
	if err := transaction.QueryRowContext(ctx, `SELECT phase,status,last_legacy_resource_id FROM evidence_lineage_migration_runs WHERE id=$1 FOR UPDATE`, command.RunID).Scan(&phase, &status, &cursor); err != nil {
		return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, err
	}
	if phase != command.Phase || status != "running" || cursor != command.AfterResourceID {
		return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, fmt.Errorf("%w: evidence lineage migration cursor changed", sharedrepository.ErrConflict)
	}
	candidates, hasMore, err := queryBackfillCandidates(ctx, transaction, command.Phase, cursor, command.BatchSize)
	if err != nil {
		return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, err
	}
	result := operationsapplication.EvidenceLineageBackfillBatchResultDTO{RunID: command.RunID, LastResourceID: cursor, HasMore: hasMore}
	for _, candidate := range candidates {
		inputSHA256, digestErr := candidate.inputSHA256(command.Phase)
		if digestErr != nil {
			return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, digestErr
		}
		reason, reasonErr := normalizedMaintenanceReason(candidate.ReasonCode)
		if reasonErr != nil {
			return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, reasonErr
		}
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO evidence_lineage_migration_items (
  run_id,phase,legacy_resource_type,legacy_resource_id,input_sha256,disposition,
  target_resource_type,target_resource_id,reason_code
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, command.RunID, command.Phase, candidate.LegacyResourceType,
			candidate.ID, inputSHA256, candidate.Disposition, candidate.TargetResourceType, candidate.TargetResourceID, reason); err != nil {
			return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, err
		}
		result.ExaminedCount++
		result.LastResourceID = candidate.ID
		switch candidate.Disposition {
		case string(operationsdomain.EvidenceLineageItemReused):
			result.ReusedCount++
		case string(operationsdomain.EvidenceLineageItemCreated):
			result.CreatedCount++
		case string(operationsdomain.EvidenceLineageItemSkipped):
			result.SkippedCount++
		case string(operationsdomain.EvidenceLineageItemBlocked):
			result.BlockedCount++
		case string(operationsdomain.EvidenceLineageItemFailed):
			result.FailedCount++
		default:
			return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, fmt.Errorf("invalid evidence lineage disposition %q", candidate.Disposition)
		}
	}
	if _, err := transaction.ExecContext(ctx, `
UPDATE evidence_lineage_migration_runs
SET version=version+1,last_legacy_resource_id=$2,
    examined_count=examined_count+$3,reused_count=reused_count+$4,created_count=created_count+$5,
    skipped_count=skipped_count+$6,blocked_count=blocked_count+$7,failed_count=failed_count+$8,updated_at=now()
WHERE id=$1`, command.RunID, result.LastResourceID, result.ExaminedCount, result.ReusedCount,
		result.CreatedCount, result.SkippedCount, result.BlockedCount, result.FailedCount); err != nil {
		return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, err
	}
	if err := transaction.Commit(); err != nil {
		return operationsapplication.EvidenceLineageBackfillBatchResultDTO{}, err
	}
	return result, nil
}

func (repository *EvidenceLineageMaintenanceRepository) CompleteEvidenceLineageBackfill(ctx context.Context, command operationsapplication.CompleteEvidenceLineageBackfillCommand) (operationsapplication.EvidenceLineageMaintenanceRunDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || command.RunID <= 0 || command.LastResourceID < 0 {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, fmt.Errorf("%w: invalid evidence lineage migration completion", sharedrepository.ErrInvalidInput)
	}
	row := repository.runtime.SQL.QueryRowContext(ctx, `
UPDATE evidence_lineage_migration_runs
SET version=version+1,status='completed',completed_at=now(),updated_at=now()
WHERE id=$1 AND status='running' AND last_legacy_resource_id=$2
RETURNING id,version,phase,status,batch_size,last_legacy_resource_id,examined_count,reused_count,
          created_count,skipped_count,blocked_count,failed_count,started_at,completed_at`, command.RunID, command.LastResourceID)
	record, err := scanEvidenceLineageMigrationRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, fmt.Errorf("%w: evidence lineage migration completion changed", sharedrepository.ErrConflict)
	}
	if err != nil {
		return operationsapplication.EvidenceLineageMaintenanceRunDTO{}, err
	}
	return evidenceLineageMaintenanceRunDTOFromRecord(record), nil
}

func (repository *EvidenceLineageMaintenanceRepository) countBackfillCandidates(ctx context.Context, phase string) (int64, error) {
	table, err := evidenceLineageBackfillTable(phase)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := repository.runtime.SQL.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (repository *EvidenceLineageMaintenanceRepository) activeEvidenceLineageProducerCount(ctx context.Context) (int64, error) {
	return activeEvidenceLineageProducerCountWithExecutor(ctx, repository.runtime.SQL)
}

type evidenceLineageSQLExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func activeEvidenceLineageProducerCountWithExecutor(ctx context.Context, executor evidenceLineageSQLExecutor) (int64, error) {
	var count int64
	err := executor.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM river_job WHERE state IN ('available','running') AND kind IN (
    'collect_source','normalize_content','evaluate_relevance','cluster_content','recompute_event_heat',
    'generate_source_document','evaluate_published_document_matches','backfill_published_monitor_matches',
    'project_accepted_document_match','reconcile_knowledge','run_retention'
  ))
  + (SELECT count(*) FROM collection_runs WHERE status IN ('queued','running'))`).Scan(&count)
	return count, err
}

func lockEvidenceLineageMaintenance(ctx context.Context, transaction *sql.Tx) error {
	_, err := transaction.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('evidence-lineage-maintenance-v1'))`)
	return err
}

func validateStartEvidenceLineageBackfillRecord(command operationsapplication.StartEvidenceLineageBackfillCommand) error {
	if !operationsdomain.EvidenceLineageMigrationPhase(command.Phase).Valid() || command.BatchSize < 1 || command.BatchSize > 1000 ||
		command.OperatorID == "" || command.OperatorID == command.ReviewerID || command.ReviewerID == "" {
		return fmt.Errorf("%w: invalid evidence lineage migration run", sharedrepository.ErrInvalidInput)
	}
	for _, digest := range []string{command.BinarySHA256, command.SchemaSHA256, command.ConfigurationSHA256, command.BackupEvidenceSHA256, command.RehearsalEvidenceSHA256} {
		if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
			return fmt.Errorf("%w: invalid evidence lineage migration digest", sharedrepository.ErrInvalidInput)
		}
	}
	return nil
}

func scanEvidenceLineageMigrationRun(row interface{ Scan(...any) error }) (evidenceLineageMigrationRunRecord, error) {
	var record evidenceLineageMigrationRunRecord
	err := row.Scan(&record.ID, &record.Version, &record.Phase, &record.Status, &record.BatchSize,
		&record.LastResourceID, &record.ExaminedCount, &record.ReusedCount, &record.CreatedCount,
		&record.SkippedCount, &record.BlockedCount, &record.FailedCount, &record.StartedAt, &record.CompletedAt)
	return record, err
}

func evidenceLineageBackfillTable(phase string) (string, error) {
	switch operationsdomain.EvidenceLineageMigrationPhase(phase) {
	case operationsdomain.MigrationPhaseSource:
		return "source_connections", nil
	case operationsdomain.MigrationPhaseEvidenceMetadata:
		return "contents", nil
	case operationsdomain.MigrationPhaseDocument:
		return "source_observations", nil
	case operationsdomain.MigrationPhaseMatch:
		return "document_match_decisions", nil
	case operationsdomain.MigrationPhaseFamilyEvent, operationsdomain.MigrationPhaseEvidenceState:
		return "events", nil
	default:
		return "", fmt.Errorf("%w: unsupported evidence lineage migration phase", sharedrepository.ErrInvalidInput)
	}
}

func queryBackfillCandidates(ctx context.Context, transaction *sql.Tx, phase string, afterID int64, batchSize int) ([]evidenceLineageBackfillCandidateRecord, bool, error) {
	query, err := evidenceLineageBackfillCandidateQuery(phase)
	if err != nil {
		return nil, false, err
	}
	rows, err := transaction.QueryContext(ctx, query, afterID, batchSize+1)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	records := make([]evidenceLineageBackfillCandidateRecord, 0, batchSize+1)
	for rows.Next() {
		var record evidenceLineageBackfillCandidateRecord
		var fingerprint string
		var targetType sql.NullString
		var targetID sql.NullInt64
		if err := rows.Scan(&record.ID, &record.LegacyResourceType, &fingerprint, &record.Disposition, &targetType, &targetID, &record.ReasonCode); err != nil {
			return nil, false, err
		}
		record.FingerprintParts = []string{fingerprint}
		if targetType.Valid {
			record.TargetResourceType = &targetType.String
		}
		if targetID.Valid {
			record.TargetResourceID = &targetID.Int64
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(records) > batchSize
	if hasMore {
		records = records[:batchSize]
	}
	return records, hasMore, nil
}

func evidenceLineageBackfillCandidateQuery(phase string) (string, error) {
	switch operationsdomain.EvidenceLineageMigrationPhase(phase) {
	case operationsdomain.MigrationPhaseSource:
		return `SELECT id,'source_connection',concat_ws(E'\x1f',version::text,source_type,name,endpoint,coalesce(terms_policy_url,'')),'reused','source_connection',id,'current_source_connection' FROM source_connections WHERE id>$1 ORDER BY id LIMIT $2`, nil
	case operationsdomain.MigrationPhaseEvidenceMetadata:
		return `SELECT id,'legacy_content',concat_ws(E'\x1f',source_connection_id::text,external_id,dedupe_key,coalesce(fetched_at::text,'')),'skipped',NULL,NULL,'legacy_body_not_raw_evidence' FROM contents WHERE id>$1 ORDER BY id LIMIT $2`, nil
	case operationsdomain.MigrationPhaseDocument:
		return `SELECT observation.id,'source_observation',concat_ws(E'\x1f',observation.source_connection_id::text,observation.external_id,observation.upstream_identity),CASE WHEN version.id IS NULL THEN 'blocked' ELSE 'reused' END,CASE WHEN version.id IS NULL THEN NULL ELSE 'document_version' END,version.id,CASE WHEN version.id IS NULL THEN 'no_authorized_document_version' ELSE 'current_document_version' END FROM source_observations observation LEFT JOIN LATERAL (SELECT id FROM document_versions WHERE source_observation_id=observation.id ORDER BY revision_no DESC,id DESC LIMIT 1) version ON true WHERE observation.id>$1 ORDER BY observation.id LIMIT $2`, nil
	case operationsdomain.MigrationPhaseMatch:
		return `SELECT id,'document_match_decision',concat_ws(E'\x1f',monitor_version_id::text,document_version_id::text,input_hash),'reused','document_match_decision',id,'current_document_match_decision' FROM document_match_decisions WHERE id>$1 ORDER BY id LIMIT $2`, nil
	case operationsdomain.MigrationPhaseFamilyEvent:
		return `SELECT id,'legacy_event',concat_ws(E'\x1f',event_key,lifecycle_status,version::text),'skipped',NULL,NULL,'legacy_event_semantics_quarantined' FROM events WHERE id>$1 ORDER BY id LIMIT $2`, nil
	case operationsdomain.MigrationPhaseEvidenceState:
		return `SELECT id,'legacy_event_evidence',concat_ws(E'\x1f',event_key,lifecycle_status,version::text),'skipped',NULL,NULL,'legacy_truth_semantics_quarantined' FROM events WHERE id>$1 ORDER BY id LIMIT $2`, nil
	default:
		return "", fmt.Errorf("%w: unsupported evidence lineage migration phase", sharedrepository.ErrInvalidInput)
	}
}
