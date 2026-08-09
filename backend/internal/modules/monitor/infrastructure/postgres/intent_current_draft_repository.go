package postgres

import (
	"context"
	"database/sql"
	"errors"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

func (repository *IntentRepository) FindCurrent(ctx context.Context, query monitorapplication.ReadCurrentIntentDraftRepositoryQuery) (monitorapplication.IntentDraftDTO, error) {
	if repository == nil || repository.runtime == nil || query.MonitorID <= 0 {
		return monitorapplication.IntentDraftDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var result monitorapplication.IntentDraftDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		record, err := lockCurrentIntentDraft(transactionCtx, executor, query.MonitorID)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentDraftNotFound
		}
		if err != nil {
			return err
		}
		result, err = readIntentDraftAt(transactionCtx, executor, record.MonitorID, record.ID, record.ResourceVersion)
		return err
	})
	if err != nil {
		return monitorapplication.IntentDraftDTO{}, mapIntentDatabaseError(err)
	}
	return result, nil
}

func (repository *IntentRepository) InitializeCurrent(ctx context.Context, mutation monitorapplication.InitializeCurrentIntentDraftMutationDTO) (monitorapplication.IntentDraftDTO, error) {
	initial := mutation.Initial
	if repository == nil || repository.runtime == nil || initial.MonitorID <= 0 || initial.DraftID != 0 ||
		initial.ResourceVersion != 1 || initial.Objective == "" || len(initial.Candidates) != 0 {
		return monitorapplication.IntentDraftDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var result monitorapplication.IntentDraftDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		configVersionID, err := lockCurrentIntentConfiguration(transactionCtx, executor, initial.MonitorID)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentDraftNotFound
		}
		if err != nil {
			return err
		}
		var existingID int64
		err = executor.QueryRowContext(transactionCtx, `
SELECT id FROM monitor_intent_drafts
WHERE monitor_id=$1 AND config_version_id=$2 FOR UPDATE`, initial.MonitorID, configVersionID).Scan(&existingID)
		if err == nil {
			return monitorapplication.ErrIntentVersionConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := executor.QueryRowContext(transactionCtx, `
INSERT INTO monitor_intent_drafts (monitor_id,config_version_id,resource_version)
VALUES ($1,$2,1) RETURNING id`, initial.MonitorID, configVersionID).Scan(&initial.DraftID); err != nil {
			return err
		}
		if _, err := insertIntentDraftRevision(transactionCtx, executor, configVersionID, initial); err != nil {
			return err
		}
		result, err = readIntentDraftAt(transactionCtx, executor, initial.MonitorID, initial.DraftID, 1)
		return err
	})
	if err != nil {
		return monitorapplication.IntentDraftDTO{}, mapIntentDatabaseError(err)
	}
	return result, nil
}

func (repository *IntentRepository) AuthorizeIntentControl(ctx context.Context, query monitorapplication.AuthorizeIntentControlQueryDTO) error {
	if repository == nil || repository.runtime == nil || query.ActorUserID <= 0 || query.MonitorID <= 0 || !validIntentControlOperation(query.Operation) {
		return monitorapplication.ErrIntentAuthorizationDenied
	}
	requiredRole := "editor"
	if query.Operation == monitorapplication.IntentControlSubmitExpansion || query.Operation == monitorapplication.IntentControlReviewCandidate {
		requiredRole = "admin"
	}
	var allowed bool
	err := repository.intentExecutor(ctx).QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM users u
  JOIN monitors m ON m.id=$2 AND m.deleted_at IS NULL
  WHERE u.id=$1 AND u.status='active' AND u.deleted_at IS NULL
    AND ($3='admin' AND u.role='admin' OR $3='editor' AND u.role IN ('editor','admin'))
)`, query.ActorUserID, query.MonitorID, requiredRole).Scan(&allowed)
	if err != nil {
		return mapIntentDatabaseError(err)
	}
	if !allowed {
		return monitorapplication.ErrIntentAuthorizationDenied
	}
	return nil
}

func lockCurrentIntentConfiguration(ctx context.Context, executor intentExecutor, monitorID int64) (int64, error) {
	var configVersionID int64
	err := executor.QueryRowContext(ctx, `
SELECT c.id
FROM monitors m
JOIN monitor_config_versions c
  ON c.id=m.draft_config_version_id AND c.monitor_id=m.id AND c.state='draft'
WHERE m.id=$1 AND m.deleted_at IS NULL
FOR UPDATE OF m,c`, monitorID).Scan(&configVersionID)
	return configVersionID, err
}

func lockCurrentIntentDraft(ctx context.Context, executor intentExecutor, monitorID int64) (intentDraftRecord, error) {
	var record intentDraftRecord
	err := executor.QueryRowContext(ctx, `
SELECT d.id,d.resource_version,d.monitor_id,d.config_version_id,d.created_at,d.updated_at
FROM monitors m
JOIN monitor_config_versions c
  ON c.id=m.draft_config_version_id AND c.monitor_id=m.id AND c.state='draft'
JOIN monitor_intent_drafts d
  ON d.monitor_id=m.id AND d.config_version_id=c.id
WHERE m.id=$1 AND m.deleted_at IS NULL
FOR UPDATE OF m,c,d`, monitorID).Scan(
		&record.ID, &record.ResourceVersion, &record.MonitorID, &record.ConfigVersionID,
		&record.CreatedAt, &record.UpdatedAt,
	)
	return record, err
}

func validIntentControlOperation(operation monitorapplication.IntentControlOperation) bool {
	switch operation {
	case monitorapplication.IntentControlReadDraft,
		monitorapplication.IntentControlReplaceDraft,
		monitorapplication.IntentControlSubmitExpansion,
		monitorapplication.IntentControlReadExpansion,
		monitorapplication.IntentControlReviewCandidate,
		monitorapplication.IntentControlSubmitPreview,
		monitorapplication.IntentControlReadPreview:
		return true
	default:
		return false
	}
}
