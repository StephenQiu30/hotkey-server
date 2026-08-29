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

type BackupRunRepository struct {
	runtime *database.Runtime
}

func NewBackupRunRepository(runtime *database.Runtime) *BackupRunRepository {
	return &BackupRunRepository{runtime: runtime}
}

func (repository *BackupRunRepository) RecordBackupRun(ctx context.Context, command operationsapplication.BackupRunCommand) (operationsapplication.BackupRunReceiptDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return operationsapplication.BackupRunReceiptDTO{}, sharedrepository.ErrUnavailable
	}
	var runID int64
	err := repository.runtime.SQL.QueryRowContext(ctx, `
INSERT INTO backup_runs(
  run_sha256,manifest_sha256,git_revision,status,recovery_point_at,started_at,completed_at,failure_code,asset_count
) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9)
ON CONFLICT (run_sha256) DO NOTHING
RETURNING id`, command.RunSHA256, command.ManifestSHA256, command.GitRevision, command.Status,
		command.RecoveryPointAt, command.StartedAt, command.CompletedAt, command.FailureCode, command.AssetCount).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		var manifestSHA256, status string
		if err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT id,manifest_sha256,status FROM backup_runs WHERE run_sha256=$1`, command.RunSHA256).Scan(&runID, &manifestSHA256, &status); err != nil {
			return operationsapplication.BackupRunReceiptDTO{}, databaserepository.MapError(err)
		}
		if manifestSHA256 != command.ManifestSHA256 || status != command.Status {
			return operationsapplication.BackupRunReceiptDTO{}, sharedrepository.ErrConflict
		}
	} else if err != nil {
		return operationsapplication.BackupRunReceiptDTO{}, databaserepository.MapError(err)
	}
	return operationsapplication.BackupRunReceiptDTO{RunID: runID, RunSHA256: command.RunSHA256, Status: command.Status}, nil
}

var _ operationsapplication.BackupRunStore = (*BackupRunRepository)(nil)
