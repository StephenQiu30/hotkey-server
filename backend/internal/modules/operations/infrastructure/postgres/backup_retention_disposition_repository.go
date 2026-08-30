package postgres

import (
	"context"
	"database/sql"
	"errors"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type BackupRetentionDispositionRepository struct {
	runtime *database.Runtime
}

func NewBackupRetentionDispositionRepository(runtime *database.Runtime) *BackupRetentionDispositionRepository {
	return &BackupRetentionDispositionRepository{runtime: runtime}
}

func (repository *BackupRetentionDispositionRepository) RecordBackupRetentionDisposition(ctx context.Context, command operationsapplication.BackupRetentionDispositionCommand) (operationsapplication.BackupRetentionDispositionReceiptDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return operationsapplication.BackupRetentionDispositionReceiptDTO{}, sharedrepository.ErrUnavailable
	}
	var dispositionID int64
	err := repository.runtime.SQL.QueryRowContext(ctx, `
INSERT INTO backup_retention_dispositions(
  disposition_sha256,manifest_sha256,backup_run_id,backup_run_sha256,deletion_evidence_sha256,
  status,reason_code,operator_record_id,reviewer_record_id,disposed_at
)
SELECT $1,$2,backup.id,backup.run_sha256,$4,'disposed',$5,$6,$7,$8
FROM backup_runs AS backup
WHERE backup.run_sha256=$3 AND backup.status='succeeded' AND backup.completed_at <= $8
ON CONFLICT (disposition_sha256) DO NOTHING
RETURNING id`, command.DispositionSHA256, command.ManifestSHA256, command.BackupRunSHA256,
		command.DeletionEvidenceSHA256, command.ReasonCode, command.OperatorID, command.ReviewerID,
		command.DisposedAt).Scan(&dispositionID)
	if errors.Is(err, sql.ErrNoRows) {
		var manifestSHA256, backupRunSHA256, status string
		if err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT id,manifest_sha256,backup_run_sha256,status
FROM backup_retention_dispositions WHERE disposition_sha256=$1`, command.DispositionSHA256).Scan(
			&dispositionID, &manifestSHA256, &backupRunSHA256, &status,
		); errors.Is(err, sql.ErrNoRows) {
			return operationsapplication.BackupRetentionDispositionReceiptDTO{}, sharedrepository.ErrConflict
		} else if err != nil {
			return operationsapplication.BackupRetentionDispositionReceiptDTO{}, databaserepository.MapError(err)
		}
		if manifestSHA256 != command.ManifestSHA256 || backupRunSHA256 != command.BackupRunSHA256 || status != "disposed" {
			return operationsapplication.BackupRetentionDispositionReceiptDTO{}, sharedrepository.ErrConflict
		}
	} else if err != nil {
		return operationsapplication.BackupRetentionDispositionReceiptDTO{}, databaserepository.MapError(err)
	}
	return operationsapplication.BackupRetentionDispositionReceiptDTO{
		DispositionID: dispositionID, BackupRunSHA256: command.BackupRunSHA256, Status: "disposed",
	}, nil
}

var _ operationsapplication.BackupRetentionDispositionStore = (*BackupRetentionDispositionRepository)(nil)
