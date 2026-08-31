package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type RetentionRepository struct{ runtime *database.Runtime }

func NewRetentionRepository(runtime *database.Runtime) *RetentionRepository {
	return &RetentionRepository{runtime: runtime}
}

type retentionQueryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *RetentionRepository) queryer(ctx context.Context) retentionQueryer {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func (repository *RetentionRepository) List(ctx context.Context) ([]operationsdomain.RetentionPolicy, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := repository.queryer(ctx).QueryContext(ctx, `SELECT id,version,data_class,retention_days,action,enabled,description FROM retention_policies ORDER BY id`)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]operationsdomain.RetentionPolicy, 0)
	for rows.Next() {
		var policy operationsdomain.RetentionPolicy
		if err := rows.Scan(&policy.ID, &policy.Version, &policy.DataClass, &policy.RetentionDays, &policy.Action, &policy.Enabled, &policy.Description); err != nil {
			return nil, err
		}
		policy.Protected = operationsdomain.ProtectedRetentionDataClass(policy.DataClass)
		items = append(items, policy)
	}
	return items, rows.Err()
}

func (repository *RetentionRepository) Find(ctx context.Context, id int64) (operationsdomain.RetentionPolicy, error) {
	if repository == nil || repository.runtime == nil {
		return operationsdomain.RetentionPolicy{}, sharedrepository.ErrUnavailable
	}
	var policy operationsdomain.RetentionPolicy
	err := repository.queryer(ctx).QueryRowContext(ctx, `SELECT id,version,data_class,retention_days,action,enabled,description FROM retention_policies WHERE id=$1`, id).Scan(
		&policy.ID, &policy.Version, &policy.DataClass, &policy.RetentionDays, &policy.Action, &policy.Enabled, &policy.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return operationsdomain.RetentionPolicy{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return operationsdomain.RetentionPolicy{}, databaserepository.MapError(err)
	}
	policy.Protected = operationsdomain.ProtectedRetentionDataClass(policy.DataClass)
	return policy, nil
}

func (repository *RetentionRepository) CreateRun(ctx context.Context, policy operationsdomain.RetentionPolicy, cutoff time.Time, batchSize int, requestedBy int64, createdAt time.Time) (operationsdomain.RetentionRun, error) {
	if repository == nil || repository.runtime == nil {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrUnavailable
	}
	if err := validateRetentionRun(policy, cutoff, batchSize); err != nil || requestedBy <= 0 || createdAt.IsZero() {
		if err != nil {
			return operationsdomain.RetentionRun{}, err
		}
		return operationsdomain.RetentionRun{}, fmt.Errorf("%w: invalid retention preview", sharedrepository.ErrInvalidInput)
	}
	candidateIDs, hasMore, err := repository.candidateIDs(ctx, policy, cutoff, batchSize, false)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	candidateHash, err := operationsdomain.RetentionCandidateHash(policy, cutoff, batchSize, candidateIDs)
	if err != nil {
		return operationsdomain.RetentionRun{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	run := operationsdomain.RetentionRun{
		PolicyID: policy.ID, PolicyVersion: policy.Version, DataClass: policy.DataClass, Cutoff: cutoff.UTC(), BatchSize: batchSize,
		CandidateCount: int64(len(candidateIDs)), HasMore: hasMore, CandidateHash: candidateHash,
		Status: operationsdomain.RetentionRunPendingApproval, RequestedBy: requestedBy, CreatedAt: createdAt.UTC(),
	}
	err = repository.queryer(ctx).QueryRowContext(ctx, `
INSERT INTO retention_runs (
    retention_policy_id,retention_policy_version,data_class,cutoff,batch_size,candidate_count,has_more,candidate_hash,status,requested_by_user_id,created_at,updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'pending_approval',$9,$10,$10)
RETURNING id`, run.PolicyID, run.PolicyVersion, run.DataClass, run.Cutoff, run.BatchSize, run.CandidateCount, run.HasMore, run.CandidateHash, run.RequestedBy, run.CreatedAt).Scan(&run.ID)
	if err != nil {
		return operationsdomain.RetentionRun{}, databaserepository.MapError(err)
	}
	for index, candidateID := range candidateIDs {
		if _, err := repository.queryer(ctx).ExecContext(ctx, `INSERT INTO retention_run_items (retention_run_id,ordinal,candidate_id,created_at) VALUES ($1,$2,$3,$4)`, run.ID, index+1, candidateID, run.CreatedAt); err != nil {
			return operationsdomain.RetentionRun{}, databaserepository.MapError(err)
		}
	}
	return run, nil
}

func (repository *RetentionRepository) FindRun(ctx context.Context, runID int64) (operationsdomain.RetentionRun, error) {
	return repository.findRun(ctx, runID, false)
}

func (repository *RetentionRepository) ApproveRun(ctx context.Context, runID int64, candidateHash string, approvedBy int64, approvedAt time.Time) (operationsdomain.RetentionRun, error) {
	if repository == nil || repository.runtime == nil {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrUnavailable
	}
	if runID <= 0 || approvedBy <= 0 || approvedAt.IsZero() || !lowerHexDigest(candidateHash) {
		return operationsdomain.RetentionRun{}, fmt.Errorf("%w: invalid retention approval", sharedrepository.ErrInvalidInput)
	}
	current, err := repository.findRun(ctx, runID, true)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	if current.Status != operationsdomain.RetentionRunPendingApproval || current.CandidateHash != candidateHash || current.RequestedBy == approvedBy {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrConflict
	}
	row := repository.queryer(ctx).QueryRowContext(ctx, retentionRunUpdateProjection(`
UPDATE retention_runs
SET status='approved',approved_by_user_id=$3,approved_at=$4,updated_at=$4
WHERE id=$1 AND status='pending_approval' AND candidate_hash=$2 AND requested_by_user_id<>$3
RETURNING `), runID, candidateHash, approvedBy, approvedAt.UTC())
	run, err := scanRetentionRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return operationsdomain.RetentionRun{}, databaserepository.MapError(err)
	}
	return run, nil
}

func (repository *RetentionRepository) ExecuteApprovedRun(ctx context.Context, runID int64, candidateHash string, executedBy int64, executedAt time.Time) (operationsdomain.RetentionRun, error) {
	if repository == nil || repository.runtime == nil {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrUnavailable
	}
	if runID <= 0 || executedBy <= 0 || executedAt.IsZero() || !lowerHexDigest(candidateHash) {
		return operationsdomain.RetentionRun{}, fmt.Errorf("%w: invalid retention execution", sharedrepository.ErrInvalidInput)
	}
	run, err := repository.findRun(ctx, runID, true)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	if run.Status != operationsdomain.RetentionRunApproved || run.CandidateHash != candidateHash || run.ApprovedBy <= 0 || run.ApprovedBy == run.RequestedBy {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrConflict
	}
	policy, err := repository.Find(ctx, run.PolicyID)
	if err != nil || policy.Version != run.PolicyVersion || policy.DataClass != run.DataClass || policy.Protected || !policy.Enabled || policy.Action != "delete" {
		if err != nil && !errors.Is(err, sharedrepository.ErrNotFound) {
			return operationsdomain.RetentionRun{}, err
		}
		return repository.blockRun(ctx, run, operationsdomain.RetentionFailurePolicyDrift, executedBy, executedAt)
	}
	currentIDs, currentHasMore, err := repository.candidateIDs(ctx, policy, run.Cutoff, run.BatchSize, false)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	currentHash, err := operationsdomain.RetentionCandidateHash(policy, run.Cutoff, run.BatchSize, currentIDs)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	storedIDs, err := repository.runCandidateIDs(ctx, run.ID)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	if currentHash != run.CandidateHash || currentHasMore != run.HasMore || !sameCandidateIDs(currentIDs, storedIDs) {
		return repository.blockRun(ctx, run, operationsdomain.RetentionFailureCandidateDrift, executedBy, executedAt)
	}
	lockedIDs, err := repository.lockedRunCandidateIDs(ctx, policy, run)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	if !sameCandidateIDs(lockedIDs, storedIDs) {
		return repository.blockRun(ctx, run, operationsdomain.RetentionFailureCandidateDrift, executedBy, executedAt)
	}
	affected, err := repository.deleteRunCandidates(ctx, policy, run)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	if affected != run.CandidateCount {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrConflict
	}
	remaining, _, err := repository.candidateIDs(ctx, policy, run.Cutoff, 1, false)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	row := repository.queryer(ctx).QueryRowContext(ctx, retentionRunUpdateProjection(`
UPDATE retention_runs
SET status='completed',executed_by_user_id=$2,executed_at=$3,affected=$4,updated_at=$3
WHERE id=$1 AND status='approved'
RETURNING `), run.ID, executedBy, executedAt.UTC(), affected)
	completed, err := scanRetentionRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return operationsdomain.RetentionRun{}, databaserepository.MapError(err)
	}
	completed.HasMore = len(remaining) > 0
	return completed, nil
}

func (repository *RetentionRepository) findRun(ctx context.Context, runID int64, lock bool) (operationsdomain.RetentionRun, error) {
	if repository == nil || repository.runtime == nil {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrUnavailable
	}
	if runID <= 0 {
		return operationsdomain.RetentionRun{}, fmt.Errorf("%w: invalid retention run id", sharedrepository.ErrInvalidInput)
	}
	query := retentionRunProjection + ` FROM retention_runs WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	run, err := scanRetentionRun(repository.queryer(ctx).QueryRowContext(ctx, query, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return operationsdomain.RetentionRun{}, databaserepository.MapError(err)
	}
	return run, nil
}

const retentionRunProjection = `SELECT id,retention_policy_id,retention_policy_version,data_class,cutoff,batch_size,candidate_count,has_more,candidate_hash,status,requested_by_user_id,approved_by_user_id,executed_by_user_id,affected,failure_code,created_at,approved_at,executed_at`

func retentionRunUpdateProjection(prefix string) string {
	return prefix + `id,retention_policy_id,retention_policy_version,data_class,cutoff,batch_size,candidate_count,has_more,candidate_hash,status,requested_by_user_id,approved_by_user_id,executed_by_user_id,affected,failure_code,created_at,approved_at,executed_at`
}

type retentionScanner interface{ Scan(...any) error }

func scanRetentionRun(scanner retentionScanner) (operationsdomain.RetentionRun, error) {
	var run operationsdomain.RetentionRun
	var approvedBy, executedBy sql.NullInt64
	var failureCode sql.NullString
	var approvedAt, executedAt sql.NullTime
	err := scanner.Scan(&run.ID, &run.PolicyID, &run.PolicyVersion, &run.DataClass, &run.Cutoff, &run.BatchSize, &run.CandidateCount, &run.HasMore, &run.CandidateHash, &run.Status, &run.RequestedBy, &approvedBy, &executedBy, &run.Affected, &failureCode, &run.CreatedAt, &approvedAt, &executedAt)
	if err != nil {
		return operationsdomain.RetentionRun{}, err
	}
	if approvedBy.Valid {
		run.ApprovedBy = approvedBy.Int64
	}
	if executedBy.Valid {
		run.ExecutedBy = executedBy.Int64
	}
	if failureCode.Valid {
		run.FailureCode = failureCode.String
	}
	if approvedAt.Valid {
		run.ApprovedAt = approvedAt.Time.UTC()
	}
	if executedAt.Valid {
		run.ExecutedAt = executedAt.Time.UTC()
	}
	run.Cutoff, run.CreatedAt = run.Cutoff.UTC(), run.CreatedAt.UTC()
	return run, nil
}

func (repository *RetentionRepository) candidateIDs(ctx context.Context, policy operationsdomain.RetentionPolicy, cutoff time.Time, batchSize int, lock bool) ([]int64, bool, error) {
	if err := validateRetentionRun(policy, cutoff, batchSize); err != nil {
		return nil, false, err
	}
	table, predicate, order, err := retentionCandidate(policy)
	if err != nil {
		return nil, false, err
	}
	query := fmt.Sprintf("SELECT id FROM %s WHERE %s ORDER BY %s LIMIT $2", table, predicate, order)
	if lock {
		query += " FOR UPDATE"
	}
	rows, err := repository.queryer(ctx).QueryContext(ctx, query, cutoff.UTC(), batchSize+1)
	if err != nil {
		return nil, false, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, batchSize+1)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, false, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(ids) > batchSize
	if hasMore {
		ids = ids[:batchSize]
	}
	return ids, hasMore, nil
}

func (repository *RetentionRepository) runCandidateIDs(ctx context.Context, runID int64) ([]int64, error) {
	rows, err := repository.queryer(ctx).QueryContext(ctx, `SELECT candidate_id FROM retention_run_items WHERE retention_run_id=$1 ORDER BY ordinal`, runID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (repository *RetentionRepository) lockedRunCandidateIDs(ctx context.Context, policy operationsdomain.RetentionPolicy, run operationsdomain.RetentionRun) ([]int64, error) {
	table, predicate, _, err := retentionCandidate(policy)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT target.id FROM %s AS target JOIN retention_run_items AS item ON item.candidate_id=target.id WHERE item.retention_run_id=$2 AND target.id IN (SELECT id FROM %s WHERE %s) ORDER BY item.ordinal FOR UPDATE OF target`, table, table, predicate)
	rows, err := repository.queryer(ctx).QueryContext(ctx, query, run.Cutoff, run.ID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0, run.CandidateCount)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (repository *RetentionRepository) deleteRunCandidates(ctx context.Context, policy operationsdomain.RetentionPolicy, run operationsdomain.RetentionRun) (int64, error) {
	table, predicate, _, err := retentionCandidate(policy)
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(`DELETE FROM %s AS target USING retention_run_items AS item WHERE item.retention_run_id=$2 AND target.id=item.candidate_id AND target.id IN (SELECT id FROM %s WHERE %s)`, table, table, predicate)
	result, err := repository.queryer(ctx).ExecContext(ctx, query, run.Cutoff, run.ID)
	if err != nil {
		return 0, databaserepository.MapError(err)
	}
	return result.RowsAffected()
}

func (repository *RetentionRepository) blockRun(ctx context.Context, run operationsdomain.RetentionRun, failureCode string, executedBy int64, executedAt time.Time) (operationsdomain.RetentionRun, error) {
	row := repository.queryer(ctx).QueryRowContext(ctx, retentionRunUpdateProjection(`
UPDATE retention_runs
SET status='blocked',executed_by_user_id=$2,executed_at=$3,failure_code=$4,updated_at=$3
WHERE id=$1 AND status='approved'
RETURNING `), run.ID, executedBy, executedAt.UTC(), failureCode)
	blocked, err := scanRetentionRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operationsdomain.RetentionRun{}, sharedrepository.ErrConflict
	}
	if err != nil {
		return operationsdomain.RetentionRun{}, databaserepository.MapError(err)
	}
	return blocked, nil
}

func sameCandidateIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func lowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func retentionCandidate(policy operationsdomain.RetentionPolicy) (table, predicate, order string, err error) {
	if policy.Protected || operationsdomain.ProtectedRetentionDataClass(policy.DataClass) {
		return "", "", "", fmt.Errorf("%w: protected retention data class", sharedrepository.ErrInvalidInput)
	}
	switch policy.DataClass {
	case "captured_items":
		return "collection_run_items", "created_at < $1 AND (content_id IS NOT NULL OR outcome <> 'captured')", "id", nil
	case "content_metric_snapshots":
		return "content_metric_snapshots", "captured_at < $1", "id", nil
	case "event_metric_snapshots":
		return "event_metric_snapshots", "captured_at < $1", "id", nil
	case "sessions":
		return "auth_sessions", "absolute_expires_at < $1 OR (revoked_at IS NOT NULL AND revoked_at < $1)", "id", nil
	case "job_attempts":
		return "river_job_attempt", "created_at < $1", "id", nil
	default:
		return "", "", "", fmt.Errorf("%w: unsupported retention data class %q", sharedrepository.ErrInvalidInput, policy.DataClass)
	}
}

func validateRetentionRun(policy operationsdomain.RetentionPolicy, cutoff time.Time, batchSize int) error {
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	if cutoff.IsZero() || batchSize < 1 || batchSize > 1000 {
		return fmt.Errorf("%w: invalid retention boundary", sharedrepository.ErrInvalidInput)
	}
	if !policy.Enabled {
		return fmt.Errorf("%w: retention policy is disabled", sharedrepository.ErrConflict)
	}
	if policy.Action != "delete" {
		return fmt.Errorf("%w: unsupported retention action", sharedrepository.ErrInvalidInput)
	}
	return nil
}
