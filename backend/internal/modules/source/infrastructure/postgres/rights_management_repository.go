package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type RightsManagementRepository struct {
	runtime *database.Runtime
}

var _ sourceapplication.RightsManagementRepository = (*RightsManagementRepository)(nil)

func NewRightsManagementRepository(runtime *database.Runtime) *RightsManagementRepository {
	return &RightsManagementRepository{runtime: runtime}
}

func (repository *RightsManagementRepository) CreateRightsPolicy(ctx context.Context, request sourceapplication.CreateRightsPolicyRepositoryDTO) (sourceapplication.CreateRightsPolicyRepositoryResultDTO, error) {
	if !repository.available() {
		return sourceapplication.CreateRightsPolicyRepositoryResultDTO{}, sharedrepository.ErrUnavailable
	}
	var result sourceapplication.CreateRightsPolicyRepositoryResultDTO
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, executor rightsManagementExecutor) error {
		record, inserted, err := insertRightsManagementPolicy(transactionCtx, executor, request)
		if err != nil {
			return err
		}
		if !inserted && record.CommandFingerprint != request.CommandFingerprint {
			return fmt.Errorf("%w: rights policy idempotency key has different input", sharedrepository.ErrConflict)
		}
		result = sourceapplication.CreateRightsPolicyRepositoryResultDTO{
			Policy: record.applicationDTO(), IdempotentReplay: !inserted,
		}
		return nil
	})
	if err != nil {
		return sourceapplication.CreateRightsPolicyRepositoryResultDTO{}, err
	}
	return result, nil
}

func (repository *RightsManagementRepository) FindRightsPolicy(ctx context.Context, query sourceapplication.FindRightsPolicyQueryDTO) (sourceapplication.RightsPolicyDTO, error) {
	if !repository.available() {
		return sourceapplication.RightsPolicyDTO{}, sharedrepository.ErrUnavailable
	}
	if query.PolicyID <= 0 || query.ExpectedVersion <= 0 {
		return sourceapplication.RightsPolicyDTO{}, fmt.Errorf("%w: rights policy identity is invalid", sharedrepository.ErrInvalidInput)
	}
	record, err := scanRightsManagementPolicy(repository.queryRow(ctx, `
SELECT `+rightsManagementPolicyColumns+`
FROM source_rights_policies
WHERE id=$1 AND version=$2`, query.PolicyID, query.ExpectedVersion))
	if err != nil {
		return sourceapplication.RightsPolicyDTO{}, mapRightsManagementDatabaseError(err)
	}
	return record.applicationDTO(), nil
}

func (repository *RightsManagementRepository) FindRightsDecision(ctx context.Context, decisionID int64) (sourceapplication.RightsDecisionDTO, error) {
	if !repository.available() {
		return sourceapplication.RightsDecisionDTO{}, sharedrepository.ErrUnavailable
	}
	if decisionID <= 0 {
		return sourceapplication.RightsDecisionDTO{}, fmt.Errorf("%w: rights decision identity is invalid", sharedrepository.ErrInvalidInput)
	}
	record, err := scanRightsManagementDecision(repository.queryRow(ctx, `
SELECT `+rightsManagementDecisionColumns+`
FROM source_rights_decisions
WHERE id=$1`, decisionID))
	if err != nil {
		return sourceapplication.RightsDecisionDTO{}, mapRightsManagementDatabaseError(err)
	}
	result, err := record.applicationDTO()
	if err != nil {
		return sourceapplication.RightsDecisionDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	return result, nil
}

func (repository *RightsManagementRepository) RecordRightsDecisions(ctx context.Context, request sourceapplication.RecordRightsDecisionRepositoryDTO) (sourceapplication.RecordRightsDecisionRepositoryResultDTO, error) {
	if !repository.available() {
		return sourceapplication.RecordRightsDecisionRepositoryResultDTO{}, sharedrepository.ErrUnavailable
	}
	var result sourceapplication.RecordRightsDecisionRepositoryResultDTO
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, executor rightsManagementExecutor) error {
		batch, inserted, err := insertRightsManagementDecisionBatch(transactionCtx, executor, request)
		if err != nil {
			return err
		}
		if !inserted {
			if batch.CommandFingerprint != request.CommandFingerprint {
				return fmt.Errorf("%w: rights decision idempotency key has different input", sharedrepository.ErrConflict)
			}
			decisions, err := readRightsManagementDecisionBatch(transactionCtx, executor, batch.ID)
			if err != nil {
				return err
			}
			if batch.DecisionCount != len(decisions) {
				return fmt.Errorf("%w: rights decision batch receipt is incomplete", sharedrepository.ErrConstraint)
			}
			result = sourceapplication.RecordRightsDecisionRepositoryResultDTO{
				DecisionBatchID: batch.ID, Decisions: decisions, IdempotentReplay: true,
			}
			return nil
		}

		decisions := make([]sourceapplication.RightsDecisionDTO, 0, len(request.Decisions))
		for _, candidate := range request.Decisions {
			decision, err := insertRightsManagementDecision(transactionCtx, executor, batch.ID, request, candidate)
			if err != nil {
				return err
			}
			decisions = append(decisions, decision)
		}
		result = sourceapplication.RecordRightsDecisionRepositoryResultDTO{
			DecisionBatchID: batch.ID, Decisions: decisions,
		}
		return nil
	})
	if err != nil {
		return sourceapplication.RecordRightsDecisionRepositoryResultDTO{}, err
	}
	return result, nil
}

type rightsManagementExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func insertRightsManagementPolicy(ctx context.Context, executor rightsManagementExecutor, request sourceapplication.CreateRightsPolicyRepositoryDTO) (rightsManagementPolicyRecord, bool, error) {
	record, err := scanRightsManagementPolicy(executor.QueryRowContext(ctx, `
INSERT INTO source_rights_policies (
  recorded_by_user_id,idempotency_key,command_fingerprint,source_connection_id,
  scope_type,scope_subject,policy_revision,priority,basis_summary,terms_url,license_uri,
  policy_hash,parent_policy_id,approved_by_user_id,effective_at,expires_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),NULLIF($11,''),$12,$13,$14,$15,$16)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING `+rightsManagementPolicyColumns,
		request.ActorID, request.IdempotencyKey, request.CommandFingerprint, request.SourceConnectionID,
		request.ScopeType, request.ScopeSubject, request.Revision, request.Priority, request.BasisSummary,
		request.TermsURL, request.LicenseURI, request.PolicyHash, request.ParentPolicyID, request.ApprovedByUserID,
		request.EffectiveFrom.UTC(), request.ExpiresAt,
	))
	if err == nil {
		return record, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return rightsManagementPolicyRecord{}, false, mapRightsManagementDatabaseError(err)
	}
	record, err = scanRightsManagementPolicy(executor.QueryRowContext(ctx, `
SELECT `+rightsManagementPolicyColumns+`
FROM source_rights_policies WHERE idempotency_key=$1`, request.IdempotencyKey))
	if err != nil {
		return rightsManagementPolicyRecord{}, false, mapRightsManagementDatabaseError(err)
	}
	return record, false, nil
}

func insertRightsManagementDecisionBatch(ctx context.Context, executor rightsManagementExecutor, request sourceapplication.RecordRightsDecisionRepositoryDTO) (rightsManagementDecisionBatchRecord, bool, error) {
	record, err := scanRightsManagementDecisionBatch(executor.QueryRowContext(ctx, `
INSERT INTO source_rights_decision_batches (
  source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
  recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING `+rightsManagementDecisionBatchColumns,
		request.SourceConnectionID, request.Policy.ID, request.ExpectedPolicyVersion,
		request.SubjectType, request.SubjectKey, request.InputDigest, request.ActorID,
		request.IdempotencyKey, request.CommandFingerprint, len(request.Decisions),
	))
	if err == nil {
		return record, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return rightsManagementDecisionBatchRecord{}, false, mapRightsManagementDatabaseError(err)
	}
	record, err = scanRightsManagementDecisionBatch(executor.QueryRowContext(ctx, `
SELECT `+rightsManagementDecisionBatchColumns+`
FROM source_rights_decision_batches WHERE idempotency_key=$1`, request.IdempotencyKey))
	if err != nil {
		return rightsManagementDecisionBatchRecord{}, false, mapRightsManagementDatabaseError(err)
	}
	return record, false, nil
}

func insertRightsManagementDecision(ctx context.Context, executor rightsManagementExecutor, batchID int64, request sourceapplication.RecordRightsDecisionRepositoryDTO, candidate sourceapplication.RightsActionDecisionDTO) (sourceapplication.RightsDecisionDTO, error) {
	reasonCodes, err := json.Marshal(candidate.ReasonCodes)
	if err != nil {
		return sourceapplication.RightsDecisionDTO{}, fmt.Errorf("encode rights decision reason codes: %w", err)
	}
	record, err := scanRightsManagementDecision(executor.QueryRowContext(ctx, `
INSERT INTO source_rights_decisions (
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,terms_url,license_uri,subject_type,subject_key,input_digest,
  action,decision,reason_codes,evaluator,evaluated_at,effective_from,expires_at,retention_days,supersedes_decision_id
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),$11,$12,$13,$14,$15,
  ARRAY(SELECT jsonb_array_elements_text($16::jsonb)),$17,$18,$19,$20,$21,$22
)
RETURNING `+rightsManagementDecisionColumns,
		batchID, request.SourceConnectionID, request.Policy.ID, request.Policy.Revision,
		request.Policy.ScopeType, request.Policy.ScopeSubject, request.Policy.Priority, request.Policy.BasisSummary,
		request.Policy.TermsURL, request.Policy.LicenseURI, request.SubjectType, request.SubjectKey, request.InputDigest,
		candidate.Action, candidate.Decision, string(reasonCodes), candidate.Evaluator,
		candidate.EvaluatedAt.UTC(), candidate.EffectiveFrom.UTC(), candidate.ExpiresAt,
		candidate.RetentionDays, candidate.SupersedesDecisionID,
	))
	if err != nil {
		return sourceapplication.RightsDecisionDTO{}, mapRightsManagementDatabaseError(err)
	}
	result, err := record.applicationDTO()
	if err != nil {
		return sourceapplication.RightsDecisionDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	return result, nil
}

func readRightsManagementDecisionBatch(ctx context.Context, executor rightsManagementExecutor, batchID int64) ([]sourceapplication.RightsDecisionDTO, error) {
	rows, err := executor.QueryContext(ctx, `
SELECT `+rightsManagementDecisionColumns+`
FROM source_rights_decisions
WHERE decision_batch_id=$1
ORDER BY action,id`, batchID)
	if err != nil {
		return nil, mapRightsManagementDatabaseError(err)
	}
	defer rows.Close()
	decisions := make([]sourceapplication.RightsDecisionDTO, 0, 9)
	for rows.Next() {
		record, err := scanRightsManagementDecision(rows)
		if err != nil {
			return nil, mapRightsManagementDatabaseError(err)
		}
		decision, err := record.applicationDTO()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
		}
		decisions = append(decisions, decision)
	}
	if err := rows.Err(); err != nil {
		return nil, mapRightsManagementDatabaseError(err)
	}
	return decisions, nil
}

func (repository *RightsManagementRepository) available() bool {
	return repository != nil && repository.runtime != nil && repository.runtime.SQL != nil && repository.runtime.GORM != nil
}

func (repository *RightsManagementRepository) queryRow(ctx context.Context, query string, arguments ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, arguments...)
	}
	return repository.runtime.SQL.QueryRowContext(ctx, query, arguments...)
}

func (repository *RightsManagementRepository) withTransaction(ctx context.Context, operation func(context.Context, rightsManagementExecutor) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return operation(ctx, transaction.SQL)
	}
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		return operation(transactionCtx, transaction.SQL)
	})
	return mapRightsManagementDatabaseError(err)
}

func mapRightsManagementDatabaseError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %v", sharedrepository.ErrNotFound, err)
	}
	return databaserepository.MapError(err)
}
