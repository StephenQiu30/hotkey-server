// Package postgres persists AI profile and run facts. It never invokes a
// provider and keeps the documented budget -> run advisory-lock order.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Repository struct{ runtime *database.Runtime }

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) CreateProfile(ctx context.Context, profile *intelligencedomain.ModelProfile) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || profile == nil {
		return fmt.Errorf("AI profile repository is unavailable")
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	var dailyBudget any
	if profile.DailyBudget != nil {
		dailyBudget = *profile.DailyBudget
	}
	if err := repository.queryRow(ctx, `
INSERT INTO ai_model_profiles (
  name, task_type, provider, model_name, credential_ref, model_version,
  embedding_dimensions, timeout_seconds, max_attempts, max_cost, daily_budget,
  fallback_priority, enabled
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id, version, created_at, updated_at`,
		profile.Name, string(profile.TaskType), string(profile.Provider), profile.ModelName, profile.CredentialRef, profile.ModelVersion,
		profile.EmbeddingDimensions, profile.TimeoutSeconds, profile.MaxAttempts, profile.MaxCost, dailyBudget, profile.FallbackPriority, profile.Enabled,
	).Scan(&profile.ID, &profile.Version, &profile.CreatedAt, &profile.UpdatedAt); err != nil {
		return fmt.Errorf("create AI profile: %w", err)
	}
	return nil
}

// ClaimInput holds only facts needed to create a deterministic AI run. Profile
// semantics and budget limits are always re-read under the transaction lock.
type ClaimInput struct {
	TaskType                                                            intelligencedomain.TaskType
	WorkspaceKey, SkillID, TargetType, RuntimeVersion                   string
	TargetID, TargetVersion, ModelProfileID                             int64
	OwningJobID                                                         *int64
	PromptVersion, InputSchemaVersion, SchemaVersion, ParametersVersion string
	InputHash, EvidenceSetHash                                          string
	Now                                                                 time.Time
}

type ClaimResult struct {
	Run    intelligencedomain.Run
	Reused bool
}

func (repository *Repository) Claim(ctx context.Context, input ClaimInput) (ClaimResult, error) {
	if repository == nil || repository.runtime == nil {
		return ClaimResult{}, fmt.Errorf("AI run repository is unavailable")
	}
	if !input.TaskType.Valid() || strings.TrimSpace(input.WorkspaceKey) == "" || strings.TrimSpace(input.SkillID) == "" ||
		strings.TrimSpace(input.TargetType) == "" || strings.TrimSpace(input.RuntimeVersion) == "" || input.TargetID <= 0 ||
		input.TargetVersion <= 0 || input.ModelProfileID <= 0 {
		return ClaimResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if input.OwningJobID != nil && *input.OwningJobID <= 0 {
		return ClaimResult{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if input.Now.IsZero() {
		input.Now = time.Now().UTC()
	} else {
		input.Now = input.Now.UTC()
	}
	var result ClaimResult
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		profile, err := loadProfileForClaim(ctx, transaction.SQL, input.ModelProfileID)
		if err != nil {
			return err
		}
		if profile.TaskType != input.TaskType || !profile.Enabled || profile.Deleted {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
		}
		reuseKey, err := intelligencedomain.NewReuseKey(intelligencedomain.ReuseKeyInput{
			TaskType: input.TaskType, WorkspaceKey: input.WorkspaceKey, SkillID: input.SkillID, TargetType: input.TargetType,
			TargetID: input.TargetID, TargetVersion: input.TargetVersion, RuntimeVersion: input.RuntimeVersion, ModelProfileID: input.ModelProfileID,
			ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion, PromptVersion: input.PromptVersion,
			InputSchemaVersion: input.InputSchemaVersion, SchemaVersion: input.SchemaVersion, ParametersVersion: input.ParametersVersion,
			InputHash: input.InputHash, EvidenceSetHash: input.EvidenceSetHash,
		})
		if err != nil {
			return err
		}
		budgetDay := input.Now.Format("2006-01-02")
		if err := lockBudget(ctx, transaction.SQL, profile.ID, budgetDay); err != nil {
			return err
		}
		if _, err := reclaimExpiredForBudget(ctx, transaction.SQL, profile.ID, budgetDay, input.Now); err != nil {
			return err
		}
		if err := lockRun(ctx, transaction.SQL, reuseKey); err != nil {
			return err
		}
		if run, found, err := findReusableRun(ctx, transaction.SQL, reuseKey); err != nil {
			return err
		} else if found {
			result = ClaimResult{Run: run, Reused: true}
			return nil
		}
		var inflight bool
		if err := transaction.SQL.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ai_runs WHERE reuse_key = $1 AND status IN ('queued','running','validating','retry_wait'))`, reuseKey).Scan(&inflight); err != nil {
			return fmt.Errorf("read in-flight AI run: %w", err)
		}
		if inflight {
			return intelligencedomain.NewError(intelligencedomain.CodeAIRunInProgress)
		}
		ledger, err := reserveBudget(ctx, transaction.SQL, profile, budgetDay)
		if err != nil {
			return err
		}
		lease := input.Now.Add(time.Duration(profile.TimeoutSeconds+30) * time.Second)
		run := intelligencedomain.Run{
			TaskType: input.TaskType, WorkspaceKey: input.WorkspaceKey, SkillID: input.SkillID, TargetType: input.TargetType,
			TargetID: input.TargetID, TargetVersion: input.TargetVersion, RuntimeVersion: input.RuntimeVersion, ModelProfileID: profile.ID,
			ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion, ReuseKey: reuseKey,
			Status: intelligencedomain.RunStatusQueued, ReservedCost: profile.MaxCost, LeaseExpiresAt: &lease,
			OwningJobID: input.OwningJobID,
		}
		if err := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO ai_runs (
 owning_job_id,workspace_key,skill_id,task_type,target_type,target_id,target_version,runtime_version,model_profile_id,prompt_version,schema_version,input_hash,status,
 model_profile_version,model_version,parameters_version,input_schema_version,evidence_set_hash,reuse_key,
 attempt,max_attempts,budget_day,reserved_cost,lease_expires_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'queued',$13,$14,$15,$16,$17,$18,1,$19,$20,$21,$22)
RETURNING id`,
			input.OwningJobID, input.WorkspaceKey, input.SkillID, string(input.TaskType), input.TargetType, input.TargetID, input.TargetVersion, input.RuntimeVersion,
			profile.ID, input.PromptVersion, input.SchemaVersion, input.InputHash,
			profile.Version, profile.ModelVersion, input.ParametersVersion, input.InputSchemaVersion, input.EvidenceSetHash, reuseKey,
			profile.MaxAttempts, budgetDay, profile.MaxCost, lease,
		).Scan(&run.ID); err != nil {
			return fmt.Errorf("create reserved AI run: %w", err)
		}
		if ledger != profile.MaxCost {
			return fmt.Errorf("AI budget reservation changed unexpectedly")
		}
		result = ClaimResult{Run: run}
		return nil
	})
	return result, err
}

type claimProfile struct {
	ID, Version, TimeoutSeconds, MaxAttempts int64
	TaskType                                 intelligencedomain.TaskType
	ModelVersion, MaxCost                    string
	DailyBudget                              sql.NullString
	Enabled, Deleted                         bool
}

func loadProfileForClaim(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (claimProfile, error) {
	var profile claimProfile
	if err := queryer.QueryRowContext(ctx, `
SELECT id,version,task_type,model_version,timeout_seconds,max_attempts,max_cost::text,daily_budget::text,enabled,deleted_at IS NOT NULL
FROM ai_model_profiles WHERE id = $1 FOR UPDATE`, id).Scan(
		&profile.ID, &profile.Version, &profile.TaskType, &profile.ModelVersion, &profile.TimeoutSeconds, &profile.MaxAttempts,
		&profile.MaxCost, &profile.DailyBudget, &profile.Enabled, &profile.Deleted,
	); err != nil {
		if err == sql.ErrNoRows {
			return claimProfile{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
		}
		return claimProfile{}, fmt.Errorf("lock AI profile: %w", err)
	}
	return profile, nil
}

func findReusableRun(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, reuseKey string) (intelligencedomain.Run, bool, error) {
	var run intelligencedomain.Run
	err := queryer.QueryRowContext(ctx, `
SELECT id,owning_job_id,workspace_key,skill_id,task_type,target_type,target_id,target_version,runtime_version,model_profile_id,model_profile_version,model_version,reuse_key,status,
	   structured_result,tokens,latency_ms,reserved_cost::text,cost::text,error_code,lease_expires_at
FROM ai_runs WHERE reuse_key = $1 AND status = 'succeeded'`, reuseKey).Scan(
		runScanTargets(&run)...,
	)
	if err == sql.ErrNoRows {
		return intelligencedomain.Run{}, false, nil
	}
	if err != nil {
		return intelligencedomain.Run{}, false, fmt.Errorf("read reusable AI run: %w", err)
	}
	return run, true, nil
}

// FindRun loads the bounded execution fact used by administrator recovery.
// It deliberately excludes prompts, provider payloads and credentials.
func (repository *Repository) FindRun(ctx context.Context, id int64) (intelligencedomain.Run, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return intelligencedomain.Run{}, sharedrepository.ErrUnavailable
	}
	if id <= 0 {
		return intelligencedomain.Run{}, fmt.Errorf("%w: invalid AI run id", sharedrepository.ErrInvalidInput)
	}
	var run intelligencedomain.Run
	err := repository.queryRow(ctx, `
SELECT id,owning_job_id,workspace_key,skill_id,task_type,target_type,target_id,target_version,runtime_version,model_profile_id,model_profile_version,model_version,reuse_key,status,
       structured_result,tokens,latency_ms,reserved_cost::text,cost::text,error_code,lease_expires_at
FROM ai_runs WHERE id=$1`, id).Scan(runScanTargets(&run)...)
	if errors.Is(err, sql.ErrNoRows) {
		return intelligencedomain.Run{}, fmt.Errorf("%w: AI run not found", sharedrepository.ErrNotFound)
	}
	if err != nil {
		return intelligencedomain.Run{}, fmt.Errorf("find AI run: %w", err)
	}
	return run, nil
}

// runScanTargets keeps nullable persistence details explicit while the domain
// value remains free of SQL types.
func runScanTargets(run *intelligencedomain.Run) []any {
	return []any{
		&run.ID, newRunOwningJobID(run), &run.WorkspaceKey, &run.SkillID, &run.TaskType, &run.TargetType, &run.TargetID, &run.TargetVersion, &run.RuntimeVersion,
		&run.ModelProfileID, &run.ModelProfileVersion,
		&run.ModelVersion, &run.ReuseKey, &run.Status, newRunStructuredResult(run), &run.Tokens, &run.LatencyMS, &run.ReservedCost, &run.Cost,
		newRunErrorCode(run), newRunLease(run),
	}
}

type runOwningJobIDScanner struct{ run *intelligencedomain.Run }

func newRunOwningJobID(run *intelligencedomain.Run) *runOwningJobIDScanner {
	return &runOwningJobIDScanner{run: run}
}

func (scanner *runOwningJobIDScanner) Scan(value any) error {
	if value == nil {
		scanner.run.OwningJobID = nil
		return nil
	}
	var nullable sql.NullInt64
	if err := nullable.Scan(value); err != nil {
		return err
	}
	jobID := nullable.Int64
	scanner.run.OwningJobID = &jobID
	return nil
}

type runStructuredResultScanner struct{ run *intelligencedomain.Run }

func newRunStructuredResult(run *intelligencedomain.Run) *runStructuredResultScanner {
	return &runStructuredResultScanner{run: run}
}

func (scanner *runStructuredResultScanner) Scan(value any) error {
	if value == nil {
		scanner.run.StructuredResult = nil
		return nil
	}
	var raw []byte
	switch value := value.(type) {
	case []byte:
		raw = value
	case string:
		raw = []byte(value)
	default:
		return fmt.Errorf("scan structured result: unsupported type %T", value)
	}
	if !json.Valid(raw) {
		return fmt.Errorf("scan structured result: invalid JSON")
	}
	scanner.run.StructuredResult = append(scanner.run.StructuredResult[:0], raw...)
	return nil
}

// sql.Scanner targets cannot directly populate optional domain fields, so the
// lightweight scanners below update the owning run only when PostgreSQL has a
// value. They are intentionally local to this persistence adapter.
type runErrorCodeScanner struct{ run *intelligencedomain.Run }

func newRunErrorCode(run *intelligencedomain.Run) *runErrorCodeScanner {
	return &runErrorCodeScanner{run: run}
}

func (scanner *runErrorCodeScanner) Scan(value any) error {
	if value == nil {
		scanner.run.ErrorCode = nil
		return nil
	}
	var nullable sql.NullInt64
	if err := nullable.Scan(value); err != nil {
		return err
	}
	code := int(nullable.Int64)
	scanner.run.ErrorCode = &code
	return nil
}

type runLeaseScanner struct{ run *intelligencedomain.Run }

func newRunLease(run *intelligencedomain.Run) *runLeaseScanner { return &runLeaseScanner{run: run} }

func (scanner *runLeaseScanner) Scan(value any) error {
	if value == nil {
		scanner.run.LeaseExpiresAt = nil
		return nil
	}
	var nullable sql.NullTime
	if err := nullable.Scan(value); err != nil {
		return err
	}
	lease := nullable.Time.UTC()
	scanner.run.LeaseExpiresAt = &lease
	return nil
}

// Settle turns an in-flight reservation into an immutable terminal cost. An
// actual cost above the reservation is recorded, then blocks the profile for
// the rest of that UTC day even when the profile has no configured daily cap.
// The stable 70002 code deliberately avoids exposing provider billing text.
func (repository *Repository) Settle(ctx context.Context, runID int64, actualCost string, now time.Time) (intelligencedomain.Run, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || runID <= 0 || !validPositiveDecimal(actualCost) {
		return intelligencedomain.Run{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	// Read only the lock identity before opening a transaction. The authoritative
	// values are re-read FOR UPDATE after both advisory locks are acquired.
	reference, err := repository.runReference(ctx, repository.queryer(ctx), runID, false)
	if err != nil {
		return intelligencedomain.Run{}, err
	}
	var settled intelligencedomain.Run
	err = repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockBudget(ctx, transaction.SQL, reference.ModelProfileID, reference.BudgetDay); err != nil {
			return err
		}
		if err := lockRun(ctx, transaction.SQL, reference.ReuseKey); err != nil {
			return err
		}
		locked, err := repository.runReference(ctx, transaction.SQL, runID, true)
		if err != nil {
			return err
		}
		if locked.ModelProfileID != reference.ModelProfileID || locked.BudgetDay != reference.BudgetDay || locked.ReuseKey != reference.ReuseKey {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		if !inFlight(locked.Status) {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		overage := decimalGreater(actualCost, locked.ReservedCost)
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE ai_budget_ledgers
SET reserved_cost = reserved_cost - $1::numeric,
    settled_cost = settled_cost + $2::numeric,
    overage_blocked = overage_blocked OR $3,
    updated_at = now()
WHERE model_profile_id = $4 AND budget_day = $5`, locked.ReservedCost, actualCost, overage, locked.ModelProfileID, locked.BudgetDay); err != nil {
			return fmt.Errorf("settle AI budget: %w", err)
		}
		status := intelligencedomain.RunStatusSucceeded
		var errorCode any
		if overage {
			status = intelligencedomain.RunStatusFailed
			errorCode = intelligencedomain.CodeAIBudgetExhausted
		}
		if err := transaction.SQL.QueryRowContext(ctx, `
UPDATE ai_runs
SET status = $1, cost = $2::numeric, reserved_cost = 0, error_code = $3,
    lease_expires_at = NULL, finished_at = $4
WHERE id = $5
RETURNING id,owning_job_id,workspace_key,skill_id,task_type,target_type,target_id,target_version,runtime_version,model_profile_id,model_profile_version,model_version,reuse_key,status,
		  structured_result,tokens,latency_ms,reserved_cost::text,cost::text,error_code,lease_expires_at`,
			string(status), actualCost, errorCode, now, runID,
		).Scan(runScanTargets(&settled)...); err != nil {
			return fmt.Errorf("settle AI run: %w", err)
		}
		return nil
	})
	return settled, err
}

// StructuredCompletion contains the only safe terminal payload accepted from
// application code. Provider-specific response objects and price data never
// cross this boundary.
type StructuredCompletion struct {
	RunID      int64
	Result     json.RawMessage
	Usage      intelligencedomain.Usage
	LatencyMS  int64
	FinishedAt time.Time
}

// CompleteStructured atomically stores a schema-validated result and settles
// the exact budget unit claimed for the run. The adapter intentionally does
// not infer a monetary amount from token use or model names.
func (repository *Repository) CompleteStructured(ctx context.Context, completion StructuredCompletion) (intelligencedomain.Run, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || completion.RunID <= 0 ||
		!json.Valid(completion.Result) || completion.LatencyMS < 0 {
		return intelligencedomain.Run{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	tokens, err := completion.Usage.TotalTokens()
	if err != nil {
		return intelligencedomain.Run{}, err
	}
	if completion.FinishedAt.IsZero() {
		completion.FinishedAt = time.Now().UTC()
	} else {
		completion.FinishedAt = completion.FinishedAt.UTC()
	}
	reference, err := repository.runReference(ctx, repository.queryer(ctx), completion.RunID, false)
	if err != nil {
		return intelligencedomain.Run{}, err
	}
	var completed intelligencedomain.Run
	err = repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockBudget(ctx, transaction.SQL, reference.ModelProfileID, reference.BudgetDay); err != nil {
			return err
		}
		if err := lockRun(ctx, transaction.SQL, reference.ReuseKey); err != nil {
			return err
		}
		locked, err := repository.runReference(ctx, transaction.SQL, completion.RunID, true)
		if err != nil {
			return err
		}
		if locked != reference || locked.Status != intelligencedomain.RunStatusValidating {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		if err := settleReservedBudget(ctx, transaction.SQL, locked, locked.ReservedCost); err != nil {
			return err
		}
		if err := transaction.SQL.QueryRowContext(ctx, `
UPDATE ai_runs
SET status='succeeded',structured_result=$1::jsonb,tokens=$2,cost=$3::numeric,
    latency_ms=$4,reserved_cost=0,error_code=NULL,lease_expires_at=NULL,finished_at=$5
WHERE id=$6
RETURNING id,owning_job_id,workspace_key,skill_id,task_type,target_type,target_id,target_version,runtime_version,model_profile_id,model_profile_version,model_version,reuse_key,status,
          structured_result,tokens,latency_ms,reserved_cost::text,cost::text,error_code,lease_expires_at`,
			completion.Result, tokens, locked.ReservedCost, completion.LatencyMS, completion.FinishedAt, completion.RunID,
		).Scan(runScanTargets(&completed)...); err != nil {
			return fmt.Errorf("complete structured AI run: %w", err)
		}
		return nil
	})
	return completed, err
}

func settleReservedBudget(ctx context.Context, queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, reference runReference, cost string) error {
	result, err := queryer.ExecContext(ctx, `
UPDATE ai_budget_ledgers
SET reserved_cost=reserved_cost-$1::numeric,settled_cost=settled_cost+$2::numeric,updated_at=now()
WHERE model_profile_id=$3 AND budget_day=$4 AND reserved_cost >= $1::numeric`,
		reference.ReservedCost, cost, reference.ModelProfileID, reference.BudgetDay)
	if err != nil {
		return fmt.Errorf("settle AI budget: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read AI budget settlement result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("AI budget reservation is inconsistent")
	}
	return nil
}

func (repository *Repository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}

func (repository *Repository) queryRow(ctx context.Context, query string, arguments ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, arguments...)
	}
	return repository.runtime.SQL.QueryRowContext(ctx, query, arguments...)
}

func (repository *Repository) queryRows(ctx context.Context, query string, arguments ...any) (*sql.Rows, error) {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryContext(ctx, query, arguments...)
	}
	return repository.runtime.SQL.QueryContext(ctx, query, arguments...)
}

func (repository *Repository) queryer(ctx context.Context) interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
} {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

type runReference struct {
	ModelProfileID                    int64
	BudgetDay, ReuseKey, ReservedCost string
	Status                            intelligencedomain.RunStatus
}

func (repository *Repository) runReference(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, runID int64, lock bool) (runReference, error) {
	query := `SELECT model_profile_id,budget_day::text,reuse_key,reserved_cost::text,status FROM ai_runs WHERE id = $1`
	if lock {
		query += " FOR UPDATE"
	}
	var reference runReference
	if err := queryer.QueryRowContext(ctx, query, runID).Scan(&reference.ModelProfileID, &reference.BudgetDay, &reference.ReuseKey, &reference.ReservedCost, &reference.Status); err != nil {
		if err == sql.ErrNoRows {
			return runReference{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		return runReference{}, fmt.Errorf("read AI run: %w", err)
	}
	return reference, nil
}

func lockRun(ctx context.Context, queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, reuseKey string) error {
	if _, err := queryer.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "ai-run:"+reuseKey); err != nil {
		return fmt.Errorf("lock AI run: %w", err)
	}
	return nil
}

func inFlight(status intelligencedomain.RunStatus) bool {
	return status == intelligencedomain.RunStatusQueued || status == intelligencedomain.RunStatusRunning ||
		status == intelligencedomain.RunStatusValidating || status == intelligencedomain.RunStatusRetryWait
}

func validPositiveDecimal(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE+-") || strings.Count(value, ".") > 1 {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) == 2 && len(parts[1]) > 4 {
		return false
	}
	for _, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return false
		}
	}
	rational, ok := new(big.Rat).SetString(value)
	return ok && rational.Sign() > 0
}

func decimalGreater(first, second string) bool {
	left, leftOK := new(big.Rat).SetString(first)
	right, rightOK := new(big.Rat).SetString(second)
	return !leftOK || !rightOK || left.Cmp(right) > 0
}
