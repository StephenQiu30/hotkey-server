package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// CollectionRepository owns Source's collection run, capture and checkpoint
// tables. It deliberately has no Monitor joins: immutable target inputs are
// supplied by Source's PublishedCollectionTargetReader port.
type CollectionRepository struct{ runtime *database.Runtime }

var _ domain.CollectionRepository = (*CollectionRepository)(nil)

const (
	collectionRunListDefaultLimit = 50
	collectionRunListMaximumLimit = 200
	collectionRunListFingerprint  = "collection-runs"
	collectionCaptureDefaultLimit = 100
	collectionCaptureMaximumLimit = 200
	collectionCaptureFingerprint  = "captured-items"
)

func NewCollectionRepository(runtime *database.Runtime) *CollectionRepository {
	return &CollectionRepository{runtime: runtime}
}

func (repository *CollectionRepository) CreateOrReuseRun(ctx context.Context, request domain.CollectionRequest) (domain.CollectionRun, bool, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.CollectionRun{}, false, sharedrepository.ErrUnavailable
	}
	if err := request.Validate(); err != nil {
		return domain.CollectionRun{}, false, fmt.Errorf("%w: collection request: %v", sharedrepository.ErrInvalidInput, err)
	}
	var run domain.CollectionRun
	created := false
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		requestCursor, etag, lastModified := initialRequestState(request.Targets)
		candidate, err := scanCollectionRun(transaction.SQL.QueryRowContext(ctx, `
INSERT INTO collection_runs
    (source_connection_id, query_signature, request_cursor, etag, last_modified,
     window_start, window_end, trigger_type, scheduled_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'schedule', now())
ON CONFLICT (source_connection_id, query_signature, window_start, window_end) DO NOTHING
RETURNING `+collectionRunColumns,
			request.SourceConnectionID, request.QuerySignature, nullableString(requestCursor), nullableString(etag), nullableString(lastModified),
			request.WindowStart.UTC(), request.WindowEnd.UTC()))
		if err == nil {
			run = candidate
			created = true
			for _, target := range sortedCollectionTargets(request.Targets) {
				if _, err := transaction.SQL.ExecContext(ctx, `
INSERT INTO collection_run_targets
    (collection_run_id, monitor_source_id, monitor_config_version_id)
VALUES ($1, $2, $3)`, run.ID, target.MonitorSourceID, target.MonitorConfigVersionID); err != nil {
					return sharedrepository.MapError(err)
				}
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return sharedrepository.MapError(err)
		}
		run, err = scanCollectionRun(transaction.SQL.QueryRowContext(ctx, `
SELECT `+collectionRunColumns+`
FROM collection_runs
WHERE source_connection_id = $1 AND query_signature = $2 AND window_start = $3 AND window_end = $4`,
			request.SourceConnectionID, request.QuerySignature, request.WindowStart.UTC(), request.WindowEnd.UTC()))
		if err != nil {
			return sharedrepository.MapError(err)
		}
		return nil
	})
	if err != nil {
		return domain.CollectionRun{}, false, err
	}
	return run, created, nil
}

// StartRun atomically claims a queued run or a bounded stale running run
// before the application starts I/O. A caller that observes a fresh running or
// completed run must reuse its durable state instead of issuing another fetch.
func (repository *CollectionRepository) StartRun(ctx context.Context, runID int64, staleBefore time.Time) (domain.CollectionRun, bool, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.CollectionRun{}, false, sharedrepository.ErrUnavailable
	}
	if runID <= 0 {
		return domain.CollectionRun{}, false, fmt.Errorf("%w: collection run id is required", sharedrepository.ErrInvalidInput)
	}
	var run domain.CollectionRun
	started := false
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var staleAt any
		if !staleBefore.IsZero() {
			staleAt = staleBefore.UTC()
		}
		candidate, err := scanCollectionRun(transaction.SQL.QueryRowContext(ctx, `
UPDATE collection_runs
SET status = 'running', started_at = now(), updated_at = now()
WHERE id = $1
  AND (status = 'queued'
       OR (status = 'running' AND $2::timestamptz IS NOT NULL
           AND (started_at IS NULL OR started_at <= $2)))
RETURNING `+collectionRunColumns, runID, staleAt))
		if err == nil {
			run, started = candidate, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return sharedrepository.MapError(err)
		}
		run, err = scanCollectionRun(transaction.SQL.QueryRowContext(ctx, `SELECT `+collectionRunColumns+` FROM collection_runs WHERE id = $1`, runID))
		if err != nil {
			return sharedrepository.MapError(err)
		}
		return nil
	})
	if err != nil {
		return domain.CollectionRun{}, false, err
	}
	return run, started, nil
}

// ListRuns returns only operations-safe run and target facts. In particular,
// neither this query nor its domain projection includes request state, source
// identity, query signature or any upstream connection detail.
func (repository *CollectionRepository) ListRuns(ctx context.Context, query domain.CollectionRunListQuery) (domain.CollectionRunPage, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.CollectionRunPage{}, sharedrepository.ErrUnavailable
	}
	limit, cursorID, err := collectionRunListParameters(query)
	if err != nil {
		return domain.CollectionRunPage{}, err
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT `+collectionRunSummaryColumns+`
FROM collection_runs
WHERE id > $1
ORDER BY id ASC
LIMIT $2`, cursorID, limit+1)
	if err != nil {
		return domain.CollectionRunPage{}, sharedrepository.MapError(err)
	}

	items := make([]domain.CollectionRunSummary, 0, limit+1)
	for rows.Next() {
		summary, err := scanCollectionRunSummary(rows)
		if err != nil {
			_ = rows.Close()
			return domain.CollectionRunPage{}, sharedrepository.MapError(err)
		}
		items = append(items, summary)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return domain.CollectionRunPage{}, sharedrepository.MapError(err)
	}
	if err := rows.Close(); err != nil {
		return domain.CollectionRunPage{}, sharedrepository.MapError(err)
	}
	page := domain.CollectionRunPage{Items: items}
	if len(page.Items) <= limit {
		for index := range page.Items {
			targets, err := collectionRunTargetSummaries(ctx, repository.runtime.SQL, page.Items[index].ID)
			if err != nil {
				return domain.CollectionRunPage{}, err
			}
			page.Items[index].Targets = targets
		}
		return page, nil
	}
	page.Items = page.Items[:limit]
	nextCursor, err := pagination.Encode("id", false, collectionRunListFingerprint, page.Items[len(page.Items)-1].ID)
	if err != nil {
		return domain.CollectionRunPage{}, fmt.Errorf("%w: encode collection run cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	page.NextCursor = nextCursor
	for index := range page.Items {
		targets, err := collectionRunTargetSummaries(ctx, repository.runtime.SQL, page.Items[index].ID)
		if err != nil {
			return domain.CollectionRunPage{}, err
		}
		page.Items[index].Targets = targets
	}
	return page, nil
}

// ListUnboundCaptured returns only Source-owned, durable capture facts. A
// stable item-ID cursor lets ingestion resume safely without seeing bound
// items or changing collection/target outcomes.
func (repository *CollectionRepository) ListUnboundCaptured(ctx context.Context, query domain.CapturedItemQuery) (domain.CapturedItemPage, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.CapturedItemPage{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return domain.CapturedItemPage{}, fmt.Errorf("%w: captured item query: %v", sharedrepository.ErrInvalidInput, err)
	}
	limit, cursorID, err := capturedItemListParameters(query)
	if err != nil {
		return domain.CapturedItemPage{}, err
	}
	statuses := []string{"pending"}
	if query.IncludeFailed {
		statuses = append(statuses, "failed")
	}
	rows, err := repository.queryRows(ctx, `
SELECT id, run_id, source_connection_id, captured_item
FROM collection_run_items
WHERE run_id = $1
  AND outcome = 'captured'
  AND content_id IS NULL
  AND ingestion_status = ANY($2)
  AND id > $3
ORDER BY id ASC
LIMIT $4`, query.RunID, statuses, cursorID, limit+1)
	if err != nil {
		return domain.CapturedItemPage{}, sharedrepository.MapError(err)
	}
	defer rows.Close()
	items := make([]domain.CapturedCollectionItem, 0, limit+1)
	for rows.Next() {
		var item domain.CapturedCollectionItem
		var payload []byte
		if err := rows.Scan(&item.ID, &item.RunID, &item.SourceConnectionID, &payload); err != nil {
			return domain.CapturedItemPage{}, sharedrepository.MapError(err)
		}
		captured, err := decodeCapturedItem(payload)
		if err != nil {
			return domain.CapturedItemPage{}, fmt.Errorf("%w: decode captured item: %v", sharedrepository.ErrConstraint, err)
		}
		item.Item = captured
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.CapturedItemPage{}, sharedrepository.MapError(err)
	}
	page := domain.CapturedItemPage{Items: items}
	if len(page.Items) <= limit {
		return page, nil
	}
	page.Items = page.Items[:limit]
	page.NextCursor, err = pagination.Encode("id", false, collectionCaptureFingerprint, page.Items[len(page.Items)-1].ID)
	if err != nil {
		return domain.CapturedItemPage{}, fmt.Errorf("%w: encode captured item cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return page, nil
}

// BindContent moves a pending or explicitly retried failed capture to
// succeeded. The source/content composite foreign key proves ownership
// without allowing Source SQL to query Content-owned tables.
func (repository *CollectionRepository) BindContent(ctx context.Context, binding domain.CapturedContentBinding) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := binding.Validate(); err != nil {
		return fmt.Errorf("%w: captured content binding: %v", sharedrepository.ErrInvalidInput, err)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		result, err := transaction.SQL.ExecContext(ctx, `
UPDATE collection_run_items
SET content_id = $1, ingestion_status = 'succeeded', ingestion_error_code = NULL
WHERE id = $2
  AND run_id = $3
  AND source_connection_id = $4
  AND outcome = 'captured'
  AND content_id IS NULL
  AND ingestion_status IN ('pending', 'failed')`, binding.ContentID, binding.CollectionItemID, binding.RunID, binding.SourceConnectionID)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("%w: captured item was bound or changed", sharedrepository.ErrConflict)
		}
		return nil
	})
}

// MarkIngestionFailure records a stable ingestion failure without changing
// capture outcome or target reconciliation. Failed captures are visible only
// through the explicit IncludeFailed retry query.
func (repository *CollectionRepository) MarkIngestionFailure(ctx context.Context, failure domain.CapturedIngestionFailure) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := failure.Validate(); err != nil {
		return fmt.Errorf("%w: captured ingestion failure: %v", sharedrepository.ErrInvalidInput, err)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		result, err := transaction.SQL.ExecContext(ctx, `
UPDATE collection_run_items
SET ingestion_status = 'failed', ingestion_error_code = $1
WHERE id = $2
  AND run_id = $3
  AND source_connection_id = $4
  AND outcome = 'captured'
  AND content_id IS NULL
  AND ingestion_status IN ('pending', 'failed')`, strings.TrimSpace(failure.Code), failure.CollectionItemID, failure.RunID, failure.SourceConnectionID)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("%w: captured item was bound or changed", sharedrepository.ErrConflict)
		}
		return nil
	})
}

// RetryRun only requeues a terminal failed/cancelled run. It never performs
// external I/O and therefore cannot create a duplicate fetch from an HTTP
// request; the ordinary collection scheduler claims the queued run later.
func (repository *CollectionRepository) RetryRun(ctx context.Context, runID int64) (domain.CollectionRunSummary, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.CollectionRunSummary{}, sharedrepository.ErrUnavailable
	}
	if runID <= 0 {
		return domain.CollectionRunSummary{}, fmt.Errorf("%w: collection run id is required", sharedrepository.ErrInvalidInput)
	}
	var summary domain.CollectionRunSummary
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		run, err := collectionRunForUpdate(ctx, transaction, runID)
		if err != nil {
			return err
		}
		if run.Status != domain.CollectionRunFailed && run.Status != domain.CollectionRunCancelled {
			return fmt.Errorf("%w: collection run status cannot be retried", sharedrepository.ErrConflict)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE collection_run_targets
SET target_status = 'queued', candidate_count = 0, accepted_count = 0, rejected_count = 0,
    error_code = NULL, updated_at = now()
WHERE collection_run_id = $1`, runID); err != nil {
			return sharedrepository.MapError(err)
		}
		summary, err = scanCollectionRunSummary(transaction.SQL.QueryRowContext(ctx, `
UPDATE collection_runs
SET status = 'queued', trigger_type = 'retry', scheduled_at = now(), retry_after = NULL,
    started_at = NULL, finished_at = NULL, candidate_count = 0, accepted_count = 0,
    rejected_count = 0, error_code = NULL, updated_at = now()
WHERE id = $1
RETURNING `+collectionRunSummaryColumns, runID))
		if err != nil {
			return sharedrepository.MapError(err)
		}
		targets, err := collectionRunTargetSummaries(ctx, transaction.SQL, runID)
		if err != nil {
			return err
		}
		summary.Targets = targets
		return nil
	})
	if err != nil {
		return domain.CollectionRunSummary{}, err
	}
	return summary, nil
}

// PersistSuccess makes captured source facts, per-target reconciliation and
// target checkpoints durable in one PostgreSQL transaction. Checkpoints move
// only after every captured item and target-item relation has been written.
func (repository *CollectionRepository) PersistSuccess(ctx context.Context, success domain.CollectionRunSuccess) (domain.CollectionRun, error) {
	return repository.PersistSuccessWith(ctx, success, nil)
}

// PersistSuccessWith is the transaction-aware variant used by the P0 queue
// handler. The hook runs after Source facts and checkpoints are written but
// before commit; queue.Store therefore inserts the downstream job into the
// same PostgreSQL transaction.
func (repository *CollectionRepository) PersistSuccessWith(ctx context.Context, success domain.CollectionRunSuccess, hook func(context.Context, int64) error) (domain.CollectionRun, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.CollectionRun{}, sharedrepository.ErrUnavailable
	}
	if success.RunID <= 0 || len(success.Targets) == 0 || success.CompletedAt.IsZero() {
		return domain.CollectionRun{}, fmt.Errorf("%w: collection success is incomplete", sharedrepository.ErrInvalidInput)
	}
	var completed domain.CollectionRun
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		run, err := collectionRunForUpdate(ctx, transaction, success.RunID)
		if err != nil {
			return err
		}
		if run.Status == domain.CollectionRunSucceeded {
			completed = run
			if hook != nil {
				if err := hook(ctx, completed.ID); err != nil {
					return err
				}
			}
			return nil
		}
		if run.Status != domain.CollectionRunRunning {
			return fmt.Errorf("%w: collection run is not running", sharedrepository.ErrConflict)
		}
		targets, err := collectionTargetsForUpdate(ctx, transaction, success.RunID, success.Targets)
		if err != nil {
			return err
		}
		itemIDs, err := repository.persistCapturedItems(ctx, transaction, run, success.Items)
		if err != nil {
			return err
		}
		completedAt := success.CompletedAt.UTC()
		candidateCount := int64(len(itemIDs))
		succeededTargets := 0
		for index, target := range targets {
			if !checkpointMatchesRunRequest(target.Checkpoint, run) {
				if err := persistTargetCaptureFailure(ctx, transaction, success.RunID, target, itemIDs, candidateCount, len(success.Result.Diagnostics), "checkpoint_state_mismatch"); err != nil {
					return err
				}
				continue
			}
			savepoint := fmt.Sprintf("collection_target_%d", index)
			if _, err := transaction.SQL.ExecContext(ctx, "SAVEPOINT "+savepoint); err != nil {
				return sharedrepository.MapError(err)
			}
			err := persistTargetSuccess(ctx, transaction, success.RunID, target, itemIDs, candidateCount, success.Result, run, completedAt)
			if err == nil {
				if _, releaseErr := transaction.SQL.ExecContext(ctx, "RELEASE SAVEPOINT "+savepoint); releaseErr != nil {
					return sharedrepository.MapError(releaseErr)
				}
				succeededTargets++
				continue
			}
			if _, rollbackErr := transaction.SQL.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); rollbackErr != nil {
				return sharedrepository.MapError(rollbackErr)
			}
			if _, releaseErr := transaction.SQL.ExecContext(ctx, "RELEASE SAVEPOINT "+savepoint); releaseErr != nil {
				return sharedrepository.MapError(releaseErr)
			}
			if !errors.Is(err, sharedrepository.ErrConflict) {
				return err
			}
			if failureErr := persistTargetCaptureFailure(ctx, transaction, success.RunID, target, itemIDs, candidateCount, len(success.Result.Diagnostics), "checkpoint_conflict"); failureErr != nil {
				return failureErr
			}
		}
		nextCursor := firstNonEmpty(success.Result.NextCursor, run.RequestCursor)
		etag := firstNonEmpty(success.Result.ETag, run.ETag)
		lastModified := firstNonEmpty(success.Result.LastModified, run.LastModified)
		status := domain.CollectionRunSucceeded
		var errorCode any
		acceptedCount := candidateCount
		if succeededTargets == 0 {
			status = domain.CollectionRunFailed
			errorCode = "target_capture_failed"
			acceptedCount = 0
		}
		completed, err = scanCollectionRun(transaction.SQL.QueryRowContext(ctx, `
UPDATE collection_runs
SET status = $1, next_cursor = $2, etag = $3, last_modified = $4,
    retry_after = NULL, page_count = page_count + 1, finished_at = $5,
    candidate_count = $6, accepted_count = $7, rejected_count = $8,
    error_code = $9, updated_at = now()
WHERE id = $10
RETURNING `+collectionRunColumns,
			string(status), nullableString(nextCursor), nullableString(etag), nullableString(lastModified), completedAt,
			candidateCount, acceptedCount, int64(len(success.Result.Diagnostics)), errorCode, success.RunID))
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if hook != nil {
			if err := hook(ctx, completed.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return domain.CollectionRun{}, err
	}
	return completed, nil
}

// PersistFailure retains retry metadata and target failure state while leaving
// the successful cursor untouched. It is deliberately a separate transaction
// from Fetch and from a failed success write, so no partial capture can move a
// checkpoint forward.
func (repository *CollectionRepository) PersistFailure(ctx context.Context, failure domain.CollectionRunFailure) (domain.CollectionRun, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.CollectionRun{}, sharedrepository.ErrUnavailable
	}
	if failure.RunID <= 0 || len(failure.Targets) == 0 || failure.CompletedAt.IsZero() || !failure.ErrorKind.Valid() {
		return domain.CollectionRun{}, fmt.Errorf("%w: collection failure is incomplete", sharedrepository.ErrInvalidInput)
	}
	var failed domain.CollectionRun
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		run, err := collectionRunForUpdate(ctx, transaction, failure.RunID)
		if err != nil {
			return err
		}
		if run.Status != domain.CollectionRunRunning {
			return fmt.Errorf("%w: collection run is not running", sharedrepository.ErrConflict)
		}
		targets, err := collectionTargetsForUpdate(ctx, transaction, failure.RunID, failure.Targets)
		if err != nil {
			return err
		}
		completedAt := failure.CompletedAt.UTC()
		for _, target := range targets {
			if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE collection_run_targets
SET target_status = 'failed', error_code = $1, updated_at = now()
WHERE id = $2 AND collection_run_id = $3`, string(failure.ErrorKind), target.ID, failure.RunID); err != nil {
				return sharedrepository.MapError(err)
			}
			if err := failCheckpoint(ctx, transaction, target.PublishedCollectionTarget, failure.Result.RateLimit.RetryAfter, completedAt); err != nil {
				return err
			}
		}
		failed, err = scanCollectionRun(transaction.SQL.QueryRowContext(ctx, `
UPDATE collection_runs
SET status = 'failed', retry_after = $1, finished_at = $2, error_code = $3,
    updated_at = now()
WHERE id = $4
RETURNING `+collectionRunColumns,
			failure.Result.RateLimit.RetryAfter, completedAt, string(failure.ErrorKind), failure.RunID))
		if err != nil {
			return sharedrepository.MapError(err)
		}
		return nil
	})
	if err != nil {
		return domain.CollectionRun{}, err
	}
	return failed, nil
}

type collectionPersistedTarget struct {
	domain.PublishedCollectionTarget
	ID int64
}

func collectionRunForUpdate(ctx context.Context, transaction database.Transaction, runID int64) (domain.CollectionRun, error) {
	run, err := scanCollectionRun(transaction.SQL.QueryRowContext(ctx, `SELECT `+collectionRunColumns+` FROM collection_runs WHERE id = $1 FOR UPDATE`, runID))
	if err != nil {
		return domain.CollectionRun{}, sharedrepository.MapError(err)
	}
	return run, nil
}

func collectionTargetsForUpdate(ctx context.Context, transaction database.Transaction, runID int64, supplied []domain.PublishedCollectionTarget) ([]collectionPersistedTarget, error) {
	byMonitorSource := make(map[int64]domain.PublishedCollectionTarget, len(supplied))
	for _, target := range supplied {
		if err := target.Validate(); err != nil {
			return nil, fmt.Errorf("%w: collection target: %v", sharedrepository.ErrInvalidInput, err)
		}
		if _, found := byMonitorSource[target.MonitorSourceID]; found {
			return nil, fmt.Errorf("%w: duplicate collection target", sharedrepository.ErrInvalidInput)
		}
		byMonitorSource[target.MonitorSourceID] = target
	}
	rows, err := transaction.SQL.QueryContext(ctx, `
SELECT id, monitor_source_id, monitor_config_version_id
FROM collection_run_targets
WHERE collection_run_id = $1
ORDER BY monitor_source_id ASC
FOR UPDATE`, runID)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	targets := make([]collectionPersistedTarget, 0, len(supplied))
	for rows.Next() {
		var id, monitorSourceID, configVersionID int64
		if err := rows.Scan(&id, &monitorSourceID, &configVersionID); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		target, found := byMonitorSource[monitorSourceID]
		if !found || target.MonitorConfigVersionID != configVersionID {
			return nil, fmt.Errorf("%w: collection run target does not match immutable request", sharedrepository.ErrConflict)
		}
		targets = append(targets, collectionPersistedTarget{PublishedCollectionTarget: target, ID: id})
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	if len(targets) != len(supplied) {
		return nil, fmt.Errorf("%w: collection run target set changed", sharedrepository.ErrConflict)
	}
	return targets, nil
}

func (repository *CollectionRepository) persistCapturedItems(ctx context.Context, transaction database.Transaction, run domain.CollectionRun, items []domain.CapturedItem) ([]int64, error) {
	itemIDs := make([]int64, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		payload, hash, err := encodedCapturedItem(item)
		if err != nil {
			return nil, fmt.Errorf("%w: captured item: %v", sharedrepository.ErrInvalidInput, err)
		}
		if _, found := seen[item.ExternalID]; found {
			continue
		}
		seen[item.ExternalID] = struct{}{}
		var itemID int64
		if err := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO collection_run_items
    (run_id, source_connection_id, source_code, external_id, content_type, captured_item_version,
     captured_item, payload_hash, raw_payload_disposition, outcome, observed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, 'captured', $10)
ON CONFLICT (run_id, external_id) DO UPDATE
SET external_id = collection_run_items.external_id
RETURNING id`,
			run.ID, run.SourceConnectionID, item.SourceCode, item.ExternalID, item.ContentType, item.Version, string(payload), hash,
			string(item.RawPayloadDisposition), item.ObservedAt.UTC()).Scan(&itemID); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		itemIDs = append(itemIDs, itemID)
	}
	return itemIDs, nil
}

func persistTargetSuccess(ctx context.Context, transaction database.Transaction, runID int64, target collectionPersistedTarget, itemIDs []int64, candidateCount int64, result domain.FetchResult, run domain.CollectionRun, completedAt time.Time) error {
	for _, itemID := range itemIDs {
		if _, err := transaction.SQL.ExecContext(ctx, `
INSERT INTO collection_run_target_items
    (collection_run_id, collection_run_target_id, collection_run_item_id, outcome)
VALUES ($1, $2, $3, 'captured')
ON CONFLICT (collection_run_target_id, collection_run_item_id) DO UPDATE
SET outcome = 'captured', reason_code = NULL`, runID, target.ID, itemID); err != nil {
			return sharedrepository.MapError(err)
		}
	}
	if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE collection_run_targets
SET target_status = 'succeeded', candidate_count = $1, accepted_count = $1,
    rejected_count = $2, error_code = NULL, updated_at = now()
WHERE id = $3 AND collection_run_id = $4`, candidateCount, int64(len(result.Diagnostics)), target.ID, runID); err != nil {
		return sharedrepository.MapError(err)
	}
	return advanceCheckpoint(ctx, transaction, target.PublishedCollectionTarget, runID, run, result, completedAt)
}

// A target-local failure is recorded explicitly so another target can still
// reconcile the durable source item and the skipped target can retry from its
// own unchanged checkpoint state.
func persistTargetCaptureFailure(ctx context.Context, transaction database.Transaction, runID int64, target collectionPersistedTarget, itemIDs []int64, candidateCount int64, diagnosticCount int, reasonCode string) error {
	for _, itemID := range itemIDs {
		if _, err := transaction.SQL.ExecContext(ctx, `
INSERT INTO collection_run_target_items
    (collection_run_id, collection_run_target_id, collection_run_item_id, outcome, reason_code)
VALUES ($1, $2, $3, 'failed', $4)
ON CONFLICT (collection_run_target_id, collection_run_item_id) DO UPDATE
SET outcome = 'failed', reason_code = $4`, runID, target.ID, itemID, reasonCode); err != nil {
			return sharedrepository.MapError(err)
		}
	}
	if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE collection_run_targets
SET target_status = 'failed', candidate_count = $1, accepted_count = 0,
    rejected_count = $2, error_code = $3, updated_at = now()
WHERE id = $4 AND collection_run_id = $5`, candidateCount, candidateCount+int64(diagnosticCount), reasonCode, target.ID, runID); err != nil {
		return sharedrepository.MapError(err)
	}
	return nil
}

type capturedItemPayload struct {
	Version               string                       `json:"version"`
	SourceCode            string                       `json:"source_code"`
	ExternalID            string                       `json:"external_id"`
	ContentType           string                       `json:"content_type"`
	Title                 string                       `json:"title"`
	Body                  string                       `json:"body,omitempty"`
	Language              string                       `json:"language,omitempty"`
	URL                   string                       `json:"url,omitempty"`
	Author                string                       `json:"author,omitempty"`
	PublishedAt           *time.Time                   `json:"published_at,omitempty"`
	ObservedAt            time.Time                    `json:"observed_at"`
	Metrics               domain.SourceMetrics         `json:"metrics"`
	RawPayloadDisposition domain.RawPayloadDisposition `json:"raw_payload_disposition"`
}

func encodedCapturedItem(item domain.CapturedItem) ([]byte, string, error) {
	if item.Version != domain.CapturedItemVersionV2 || item.SourceCode == "" || item.ExternalID == "" || item.ContentType == "" || item.ObservedAt.IsZero() || !item.RawPayloadDisposition.Valid() {
		return nil, "", errors.New("captured item is incomplete")
	}
	if err := item.Metrics.Validate(); err != nil {
		return nil, "", err
	}
	payload, err := json.Marshal(capturedItemPayload{
		Version: item.Version, SourceCode: item.SourceCode, ExternalID: item.ExternalID, ContentType: item.ContentType,
		Title: item.Title, Body: item.Body, Language: item.Language, URL: item.URL, Author: item.Author,
		PublishedAt: item.PublishedAt, ObservedAt: item.ObservedAt.UTC(), Metrics: item.Metrics,
		RawPayloadDisposition: item.RawPayloadDisposition,
	})
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(payload)
	return payload, hex.EncodeToString(digest[:]), nil
}

func decodeCapturedItem(payload []byte) (domain.CapturedItem, error) {
	var persisted capturedItemPayload
	if err := json.Unmarshal(payload, &persisted); err != nil {
		return domain.CapturedItem{}, err
	}
	if persisted.Version != domain.CapturedItemVersionV1 && persisted.Version != domain.CapturedItemVersionV2 {
		return domain.CapturedItem{}, fmt.Errorf("unsupported captured item version %q", persisted.Version)
	}
	if persisted.SourceCode == "" || persisted.ExternalID == "" || persisted.ContentType == "" || persisted.ObservedAt.IsZero() || !persisted.RawPayloadDisposition.Valid() {
		return domain.CapturedItem{}, errors.New("captured item is incomplete")
	}
	if err := persisted.Metrics.Validate(); err != nil {
		return domain.CapturedItem{}, err
	}
	metrics := persisted.Metrics
	if persisted.Version == domain.CapturedItemVersionV1 {
		metrics = legacyCapturedMetrics(metrics)
	}
	item := domain.CapturedItem{
		Version: persisted.Version, SourceCode: persisted.SourceCode, ExternalID: persisted.ExternalID,
		ContentType: persisted.ContentType, Title: persisted.Title, Body: persisted.Body, Language: persisted.Language,
		URL: persisted.URL, Author: persisted.Author, ObservedAt: persisted.ObservedAt.UTC(), Metrics: metrics,
		RawPayloadDisposition: persisted.RawPayloadDisposition,
	}
	if persisted.PublishedAt != nil {
		publishedAt := persisted.PublishedAt.UTC()
		item.PublishedAt = &publishedAt
	}
	return item, nil
}

func legacyCapturedMetrics(metrics domain.SourceMetrics) domain.SourceMetrics {
	return domain.SourceMetrics{
		ViewCount:    legacyMetric(metrics.ViewCount),
		LikeCount:    legacyMetric(metrics.LikeCount),
		CommentCount: legacyMetric(metrics.CommentCount),
		ShareCount:   legacyMetric(metrics.ShareCount),
	}
}

func legacyMetric(metric *int64) *int64 {
	if metric == nil || *metric == 0 {
		return nil
	}
	return domain.KnownMetric(*metric)
}

func advanceCheckpoint(ctx context.Context, transaction database.Transaction, target domain.PublishedCollectionTarget, runID int64, run domain.CollectionRun, result domain.FetchResult, completedAt time.Time) error {
	checkpoint := target.Checkpoint
	nextCursor := firstNonEmpty(result.NextCursor, run.RequestCursor)
	etag := firstNonEmpty(result.ETag, run.ETag)
	lastModified := firstNonEmpty(result.LastModified, run.LastModified)
	nextPollAt := completedAt.Add(target.CollectionInterval).UTC()
	update, err := transaction.SQL.ExecContext(ctx, `
UPDATE source_checkpoints
SET cursor_value = $1, etag = $2, last_modified = $3, last_successful_run_id = $4,
    last_fetched_at = $5, next_poll_at = $6, consecutive_failures = 0,
    version = version + 1, updated_at = now()
WHERE id = $7 AND monitor_source_id = $8 AND query_hash = $9 AND version = $10`,
		nullableString(nextCursor), nullableString(etag), nullableString(lastModified), runID, completedAt, nextPollAt,
		checkpoint.ID, target.MonitorSourceID, target.QuerySignature, checkpoint.Version)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	count, err := update.RowsAffected()
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if count != 1 {
		return fmt.Errorf("%w: collection checkpoint changed", sharedrepository.ErrConflict)
	}
	return nil
}

func failCheckpoint(ctx context.Context, transaction database.Transaction, target domain.PublishedCollectionTarget, retryAfter *time.Time, completedAt time.Time) error {
	checkpoint := target.Checkpoint
	nextPollAt := completedAt.Add(target.CollectionInterval).UTC()
	if retryAfter != nil && retryAfter.After(completedAt) {
		nextPollAt = retryAfter.UTC()
	}
	update, err := transaction.SQL.ExecContext(ctx, `
UPDATE source_checkpoints
SET next_poll_at = $1, consecutive_failures = consecutive_failures + 1,
    version = version + 1, updated_at = now()
WHERE id = $2 AND monitor_source_id = $3 AND query_hash = $4 AND version = $5`,
		nextPollAt, checkpoint.ID, target.MonitorSourceID, target.QuerySignature, checkpoint.Version)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	count, err := update.RowsAffected()
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if count != 1 {
		return fmt.Errorf("%w: collection checkpoint changed", sharedrepository.ErrConflict)
	}
	return nil
}

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func (repository *CollectionRepository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}

func (repository *CollectionRepository) queryRows(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryContext(ctx, query, args...)
	}
	return repository.runtime.SQL.QueryContext(ctx, query, args...)
}

type collectionRowsQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func collectionRunTargetSummaries(ctx context.Context, querier collectionRowsQuerier, runID int64) ([]domain.CollectionRunTargetSummary, error) {
	rows, err := querier.QueryContext(ctx, `
SELECT id, target_status, candidate_count, accepted_count, rejected_count, error_code
FROM collection_run_targets
WHERE collection_run_id = $1
ORDER BY id ASC`, runID)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	targets := make([]domain.CollectionRunTargetSummary, 0)
	for rows.Next() {
		var target domain.CollectionRunTargetSummary
		var status string
		var errorCode sql.NullString
		if err := rows.Scan(&target.ID, &status, &target.CandidateCount, &target.AcceptedCount, &target.RejectedCount, &errorCode); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		target.Status, target.ErrorCode = domain.CollectionRunStatus(status), errorCode.String
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return targets, nil
}

func collectionRunListParameters(query domain.CollectionRunListQuery) (int, int64, error) {
	limit := query.Limit
	if limit == 0 {
		limit = collectionRunListDefaultLimit
	}
	if limit < 1 || limit > collectionRunListMaximumLimit {
		return 0, 0, fmt.Errorf("%w: collection run limit must be 1-%d", sharedrepository.ErrInvalidInput, collectionRunListMaximumLimit)
	}
	cursor, err := pagination.Decode(query.Cursor, "id", false, collectionRunListFingerprint)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: collection run cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return limit, cursor.ID, nil
}

func capturedItemListParameters(query domain.CapturedItemQuery) (int, int64, error) {
	limit := query.Limit
	if limit == 0 {
		limit = collectionCaptureDefaultLimit
	}
	if limit < 1 || limit > collectionCaptureMaximumLimit {
		return 0, 0, fmt.Errorf("%w: captured item limit must be from 1 to %d", sharedrepository.ErrInvalidInput, collectionCaptureMaximumLimit)
	}
	cursor, err := pagination.Decode(query.Cursor, "id", false, collectionCaptureFingerprint)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: captured item cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return limit, cursor.ID, nil
}

// initialRequestState selects an explicit checkpoint-equivalence group. A
// blank checkpoint is always safest for newly published targets because it
// cannot apply an older target's conditional validators; otherwise the largest
// equivalence group wins with a lexical tie break. Targets in other groups are
// kept pending and never inherit this run's cursor or validators.
func initialRequestState(targets []domain.PublishedCollectionTarget) (string, string, string) {
	counts := make(map[collectionCheckpointState]int, len(targets))
	for _, target := range targets {
		state := checkpointState(target.Checkpoint)
		counts[state]++
	}
	best := collectionCheckpointState{}
	bestCount := -1
	for state, count := range counts {
		if state.blank() {
			return "", "", ""
		}
		if count > bestCount || (count == bestCount && state.key() < best.key()) {
			best, bestCount = state, count
		}
	}
	return best.cursor, best.etag, best.lastModified
}

type collectionCheckpointState struct {
	cursor, etag, lastModified string
}

func checkpointState(checkpoint domain.CollectionCheckpoint) collectionCheckpointState {
	return collectionCheckpointState{cursor: checkpoint.CursorValue, etag: checkpoint.ETag, lastModified: checkpoint.LastModified}
}

func (state collectionCheckpointState) blank() bool {
	return state.cursor == "" && state.etag == "" && state.lastModified == ""
}

func (state collectionCheckpointState) key() string {
	return state.cursor + "\x00" + state.etag + "\x00" + state.lastModified
}

func checkpointMatchesRunRequest(checkpoint domain.CollectionCheckpoint, run domain.CollectionRun) bool {
	state := checkpointState(checkpoint)
	return state.cursor == run.RequestCursor && state.etag == run.ETag && state.lastModified == run.LastModified
}

func sortedCollectionTargets(targets []domain.PublishedCollectionTarget) []domain.PublishedCollectionTarget {
	ordered := append([]domain.PublishedCollectionTarget(nil), targets...)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].MonitorSourceID < ordered[right].MonitorSourceID
	})
	return ordered
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
