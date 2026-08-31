package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func (repository *IntentRepository) ReserveAndEnqueue(ctx context.Context, request monitorapplication.ReserveIntentRunDTO) (monitorapplication.IntentRunReservationDTO, error) {
	if !validIntentRepositoryRunTask(request) {
		return monitorapplication.IntentRunReservationDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var reservation monitorapplication.IntentRunReservationDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		existing, err := findIntentRunByIdempotencyKey(transactionCtx, executor, request.IdempotencyKey, true)
		if err == nil {
			if !sameIntentReservation(existing, request) {
				return monitorapplication.ErrIntentIdempotencyConflict
			}
			reservation = monitorapplication.IntentRunReservationDTO{Run: existing.applicationDTO(), Created: false}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		draft, err := lockIntentDraft(transactionCtx, executor, request.Task.MonitorID, request.Task.DraftID)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentDraftNotFound
		}
		if err != nil {
			return err
		}
		if draft.ResourceVersion != request.Task.DraftResourceVersion {
			return monitorapplication.ErrIntentVersionConflict
		}

		var runID int64
		if err := executor.QueryRowContext(transactionCtx, `SELECT nextval(pg_get_serial_sequence('monitor_intent_analysis_runs','id'))`).Scan(&runID); err != nil {
			return err
		}
		encoded, err := encodeIntentRunJobArgs(runID, request)
		if err != nil {
			return monitorapplication.ErrInvalidIntentContract
		}
		jobID, jobCreated, err := repository.jobs.Enqueue(transactionCtx, queue.Job{
			Kind: queue.KindAnalyzeMonitorIntent, UniqueKey: "monitor-intent:" + request.IdempotencyKey,
			DurableArgs: encoded, ScheduledAt: request.RequestedAt.UTC(), MaxAttempts: 3, Priority: 3,
		})
		if err != nil {
			return err
		}
		if !jobCreated {
			existing, err = findIntentRunByRiverJob(transactionCtx, executor, jobID, true)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: existing intent River job has no run receipt", monitorapplication.ErrInvalidIntentContract)
			}
			if err != nil {
				return err
			}
			if !sameIntentReservation(existing, request) {
				return monitorapplication.ErrIntentIdempotencyConflict
			}
			reservation = monitorapplication.IntentRunReservationDTO{Run: existing.applicationDTO(), Created: false}
			return nil
		}
		record, err := scanIntentAnalysisRun(executor.QueryRowContext(transactionCtx, `
INSERT INTO monitor_intent_analysis_runs (
  id,monitor_id,draft_id,draft_resource_version,kind,input_hash,profile_version,sample_limit,
  request_hash,idempotency_key,river_job_id,status,queued_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'queued',$12)
RETURNING `+intentAnalysisRunColumns,
			runID, request.Task.MonitorID, request.Task.DraftID, request.Task.DraftResourceVersion,
			request.Task.Kind, request.Task.InputHash, request.Task.AnalysisProfile, request.Task.SampleLimit,
			request.RequestHash, request.IdempotencyKey, jobID, request.RequestedAt.UTC(),
		))
		if err != nil {
			return err
		}
		reservation = monitorapplication.IntentRunReservationDTO{Run: record.applicationDTO(), Created: true}
		return nil
	})
	if err != nil {
		return monitorapplication.IntentRunReservationDTO{}, mapIntentDatabaseError(err)
	}
	return reservation, nil
}

func (repository *IntentRepository) FindIntentAnalysisTask(ctx context.Context, query monitorapplication.ReadIntentAnalysisTaskQuery) (monitorapplication.IntentAnalysisTaskDTO, error) {
	if repository == nil || repository.runtime == nil || query.RunID <= 0 || query.DraftID <= 0 || query.DraftResourceVersion <= 0 {
		return monitorapplication.IntentAnalysisTaskDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	record, err := scanIntentAnalysisRun(repository.intentExecutor(ctx).QueryRowContext(ctx, `
SELECT `+intentAnalysisRunColumns+`
FROM monitor_intent_analysis_runs
WHERE id=$1 AND draft_id=$2 AND draft_resource_version=$3`, query.RunID, query.DraftID, query.DraftResourceVersion))
	if err != nil {
		return monitorapplication.IntentAnalysisTaskDTO{}, intentRunNotFound(err)
	}
	return monitorapplication.IntentAnalysisTaskDTO{
		Run: monitorapplication.IntentRunReferenceDTO{
			RunID: record.ID, Kind: record.Kind, MonitorID: record.MonitorID, DraftID: record.DraftID,
			DraftResourceVersion: record.DraftResourceVersion, InputHash: record.InputHash,
		},
		AnalysisProfile: record.AnalysisProfile, SampleLimit: record.SampleLimit,
	}, nil
}

func (repository *IntentRepository) FindExpansion(ctx context.Context, query monitorapplication.ReadExpansionRunQuery) (monitorapplication.ExpansionRunDTO, error) {
	if repository == nil || repository.runtime == nil || query.MonitorID <= 0 || query.DraftID <= 0 || query.DraftResourceVersion <= 0 || query.RunID <= 0 {
		return monitorapplication.ExpansionRunDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	record, err := findIntentRun(ctx, repository.intentExecutor(ctx), query.MonitorID, query.DraftID, query.DraftResourceVersion, query.RunID, "expansion", false)
	if err != nil {
		return monitorapplication.ExpansionRunDTO{}, intentRunNotFound(err)
	}
	candidates, err := readIntentRunCandidates(ctx, repository.intentExecutor(ctx), record.ID)
	if err != nil {
		return monitorapplication.ExpansionRunDTO{}, mapIntentDatabaseError(err)
	}
	return monitorapplication.ExpansionRunDTO{Run: record.applicationDTO(), Candidates: candidates}, nil
}

func (repository *IntentRepository) FindPreview(ctx context.Context, query monitorapplication.ReadPreviewRunQuery) (monitorapplication.PreviewRunDTO, error) {
	if repository == nil || repository.runtime == nil || query.MonitorID <= 0 || query.DraftID <= 0 || query.DraftResourceVersion <= 0 || query.RunID <= 0 {
		return monitorapplication.PreviewRunDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	record, err := findIntentRun(ctx, repository.intentExecutor(ctx), query.MonitorID, query.DraftID, query.DraftResourceVersion, query.RunID, "preview", false)
	if err != nil {
		return monitorapplication.PreviewRunDTO{}, intentRunNotFound(err)
	}
	preview, err := readIntentPreview(ctx, repository.intentExecutor(ctx), record.ID)
	if err != nil {
		return monitorapplication.PreviewRunDTO{}, mapIntentDatabaseError(err)
	}
	return monitorapplication.PreviewRunDTO{Run: record.applicationDTO(), Preview: preview}, nil
}

func (repository *IntentRepository) FindExpansionStatus(ctx context.Context, lookup monitorapplication.IntentRunStatusLookupDTO) (monitorapplication.ExpansionRunDTO, error) {
	if repository == nil || repository.runtime == nil || lookup.MonitorID <= 0 || lookup.RunID <= 0 {
		return monitorapplication.ExpansionRunDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	record, err := findIntentRunStatus(ctx, repository.intentExecutor(ctx), lookup.MonitorID, lookup.RunID, "expansion")
	if err != nil {
		return monitorapplication.ExpansionRunDTO{}, intentRunNotFound(err)
	}
	candidates, err := readIntentRunCandidates(ctx, repository.intentExecutor(ctx), record.ID)
	if err != nil {
		return monitorapplication.ExpansionRunDTO{}, mapIntentDatabaseError(err)
	}
	return monitorapplication.ExpansionRunDTO{Run: record.applicationDTO(), Candidates: candidates}, nil
}

func (repository *IntentRepository) FindPreviewStatus(ctx context.Context, lookup monitorapplication.IntentRunStatusLookupDTO) (monitorapplication.PreviewRunDTO, error) {
	if repository == nil || repository.runtime == nil || lookup.MonitorID <= 0 || lookup.RunID <= 0 {
		return monitorapplication.PreviewRunDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	record, err := findIntentRunStatus(ctx, repository.intentExecutor(ctx), lookup.MonitorID, lookup.RunID, "preview")
	if err != nil {
		return monitorapplication.PreviewRunDTO{}, intentRunNotFound(err)
	}
	preview, err := readIntentPreview(ctx, repository.intentExecutor(ctx), record.ID)
	if err != nil {
		return monitorapplication.PreviewRunDTO{}, mapIntentDatabaseError(err)
	}
	return monitorapplication.PreviewRunDTO{Run: record.applicationDTO(), Preview: preview}, nil
}

func (repository *IntentRepository) SaveTransition(ctx context.Context, transition monitorapplication.IntentRunTransitionDTO) (monitorapplication.IntentRunTransitionReceiptDTO, error) {
	if transition.Expected.ID <= 0 || transition.Next.ID != transition.Expected.ID {
		return monitorapplication.IntentRunTransitionReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var receipt monitorapplication.IntentRunTransitionReceiptDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		current, err := findIntentRunByID(transactionCtx, executor, transition.Expected.ID, true)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentRunNotFound
		}
		if err != nil {
			return err
		}
		currentDTO := current.applicationDTO()
		if sameIntentRunLifecycle(currentDTO, transition.Next) {
			receipt = monitorapplication.IntentRunTransitionReceiptDTO{Run: currentDTO, Changed: false}
			return nil
		}
		if !sameIntentRunLifecycle(currentDTO, transition.Expected) || !sameIntentRunIdentityDTO(transition.Expected, transition.Next) {
			return monitorapplication.ErrIntentRunStateConflict
		}
		if !validIntentWorkerTransition(transition.Expected, transition.Next) {
			return monitorapplication.ErrIntentRunStateConflict
		}
		updated, err := updateIntentRunLifecycle(transactionCtx, executor, transition.Next, "")
		if err != nil {
			return err
		}
		receipt = monitorapplication.IntentRunTransitionReceiptDTO{Run: updated.applicationDTO(), Changed: true}
		return nil
	})
	if err != nil {
		return monitorapplication.IntentRunTransitionReceiptDTO{}, mapIntentDatabaseError(err)
	}
	return receipt, nil
}

func (repository *IntentRepository) CompletePreview(ctx context.Context, mutation monitorapplication.CompletePreviewRunMutationDTO) (monitorapplication.CompletePreviewRunReceiptDTO, error) {
	if mutation.Transition.Expected.ID <= 0 || validateIntentRecordHash(mutation.ResultFingerprint) != nil {
		return monitorapplication.CompletePreviewRunReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var receipt monitorapplication.CompletePreviewRunReceiptDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		current, err := findIntentRunByID(transactionCtx, executor, mutation.Transition.Expected.ID, true)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentRunNotFound
		}
		if err != nil {
			return err
		}
		if current.ResultFingerprint.Valid {
			if current.ResultFingerprint.String != mutation.ResultFingerprint {
				return monitorapplication.ErrIntentRunResultConflict
			}
			stored, err := readIntentPreview(transactionCtx, executor, current.ID)
			if err != nil || stored == nil {
				if err == nil {
					err = monitorapplication.ErrInvalidIntentContract
				}
				return err
			}
			receipt = monitorapplication.CompletePreviewRunReceiptDTO{
				Preview:           monitorapplication.PreviewRunDTO{Run: current.applicationDTO(), Preview: stored},
				ResultFingerprint: current.ResultFingerprint.String, Changed: false,
			}
			return nil
		}
		if current.Kind != "preview" || !sameIntentRunLifecycle(current.applicationDTO(), mutation.Transition.Expected) ||
			!validIntentSuccessTransition(mutation.Transition.Expected, mutation.Transition.Next, "succeeded") {
			return monitorapplication.ErrIntentRunStateConflict
		}
		updated, err := updateIntentRunLifecycle(transactionCtx, executor, mutation.Transition.Next, mutation.ResultFingerprint)
		if err != nil {
			return err
		}
		if err := insertIntentPreview(transactionCtx, executor, updated.ID, mutation.Preview); err != nil {
			return err
		}
		stored, err := readIntentPreview(transactionCtx, executor, updated.ID)
		if err != nil || stored == nil {
			if err == nil {
				err = monitorapplication.ErrInvalidIntentContract
			}
			return err
		}
		receipt = monitorapplication.CompletePreviewRunReceiptDTO{
			Preview:           monitorapplication.PreviewRunDTO{Run: updated.applicationDTO(), Preview: stored},
			ResultFingerprint: mutation.ResultFingerprint, Changed: true,
		}
		return nil
	})
	if err != nil {
		return monitorapplication.CompletePreviewRunReceiptDTO{}, mapIntentDatabaseError(err)
	}
	return receipt, nil
}

func (repository *IntentRepository) CompleteExpansion(ctx context.Context, mutation monitorapplication.CompleteExpansionRunMutationDTO) (monitorapplication.CompleteExpansionRunReceiptDTO, error) {
	if mutation.Transition.Expected.ID <= 0 || validateIntentRecordHash(mutation.ResultFingerprint) != nil ||
		mutation.DraftMutation.Kind != monitorapplication.IntentDraftMutationExpansionResult || len(mutation.Candidates) == 0 {
		return monitorapplication.CompleteExpansionRunReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	var receipt monitorapplication.CompleteExpansionRunReceiptDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		draftRecord, err := lockIntentDraft(transactionCtx, executor, mutation.Transition.Expected.MonitorID, mutation.Transition.Expected.DraftID)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentDraftNotFound
		}
		if err != nil {
			return err
		}
		current, err := findIntentRunByID(transactionCtx, executor, mutation.Transition.Expected.ID, true)
		if errors.Is(err, sql.ErrNoRows) {
			return monitorapplication.ErrIntentRunNotFound
		}
		if err != nil {
			return err
		}
		resultVersion := current.DraftResourceVersion + 1
		if current.ResultFingerprint.Valid {
			if current.ResultFingerprint.String != mutation.ResultFingerprint {
				return monitorapplication.ErrIntentRunResultConflict
			}
			candidates, err := readIntentRunCandidates(transactionCtx, executor, current.ID)
			if err != nil {
				return err
			}
			draft, err := readIntentDraftAt(transactionCtx, executor, current.MonitorID, current.DraftID, resultVersion)
			if err != nil {
				return err
			}
			receipt = monitorapplication.CompleteExpansionRunReceiptDTO{
				Expansion: monitorapplication.ExpansionRunDTO{Run: current.applicationDTO(), Candidates: candidates},
				Draft:     draft, ResultFingerprint: current.ResultFingerprint.String, Changed: false,
			}
			return nil
		}
		if current.Kind != "expansion" || !sameIntentRunLifecycle(current.applicationDTO(), mutation.Transition.Expected) ||
			!validIntentSuccessTransition(mutation.Transition.Expected, mutation.Transition.Next, "invalidated") {
			return monitorapplication.ErrIntentRunStateConflict
		}
		if draftRecord.ResourceVersion != mutation.DraftMutation.ExpectedResourceVersion ||
			mutation.DraftMutation.ExpectedDraftID != current.DraftID || mutation.DraftMutation.Next.MonitorID != current.MonitorID ||
			mutation.DraftMutation.Next.DraftID != current.DraftID || mutation.DraftMutation.Next.ResourceVersion != resultVersion {
			return monitorapplication.ErrIntentVersionConflict
		}
		currentDraft, err := readIntentDraftAt(transactionCtx, executor, current.MonitorID, current.DraftID, current.DraftResourceVersion)
		if err != nil {
			return err
		}
		expectedNext := currentDraft
		expectedNext.ResourceVersion++
		expectedNext.Candidates = append(append([]monitorapplication.ExpansionCandidateDTO(nil), currentDraft.Candidates...), mutation.Candidates...)
		if !sameIntentDraftDTO(expectedNext, mutation.DraftMutation.Next) || mutation.DraftMutation.InvalidatedAt.IsZero() ||
			mutation.Transition.Next.InvalidatedAt == nil || !mutation.DraftMutation.InvalidatedAt.Equal(*mutation.Transition.Next.InvalidatedAt) {
			return monitorapplication.ErrInvalidIntentContract
		}
		revisionID, err := insertIntentDraftRevisionWithoutCandidates(transactionCtx, executor, draftRecord.ConfigVersionID, mutation.DraftMutation.Next)
		if err != nil {
			return err
		}
		if err := insertGeneratedIntentCandidates(transactionCtx, executor, current.ID, mutation.DraftMutation.Next, mutation.Candidates); err != nil {
			return err
		}
		if err := insertIntentDraftCandidateLinks(transactionCtx, executor, revisionID, mutation.DraftMutation.Next); err != nil {
			return err
		}
		updated, err := updateIntentRunLifecycle(transactionCtx, executor, mutation.Transition.Next, mutation.ResultFingerprint)
		if err != nil {
			return err
		}
		result, err := executor.ExecContext(transactionCtx, `
UPDATE monitor_intent_drafts SET resource_version=$3,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND monitor_id=$2 AND resource_version=$4`, current.DraftID, current.MonitorID, resultVersion, current.DraftResourceVersion)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return monitorapplication.ErrIntentVersionConflict
		}
		if err := invalidateSupersededIntentRuns(transactionCtx, executor, current.MonitorID, current.DraftID, resultVersion, *mutation.Transition.Next.InvalidatedAt, current.ID); err != nil {
			return err
		}
		candidates, err := readIntentRunCandidates(transactionCtx, executor, current.ID)
		if err != nil {
			return err
		}
		draft, err := readIntentDraftAt(transactionCtx, executor, current.MonitorID, current.DraftID, resultVersion)
		if err != nil {
			return err
		}
		receipt = monitorapplication.CompleteExpansionRunReceiptDTO{
			Expansion: monitorapplication.ExpansionRunDTO{Run: updated.applicationDTO(), Candidates: candidates},
			Draft:     draft, ResultFingerprint: mutation.ResultFingerprint, Changed: true,
		}
		return nil
	})
	if err != nil {
		return monitorapplication.CompleteExpansionRunReceiptDTO{}, mapIntentDatabaseError(err)
	}
	return receipt, nil
}

func findIntentRun(ctx context.Context, executor intentExecutor, monitorID, draftID, resourceVersion, runID int64, kind string, lock bool) (intentAnalysisRunRecord, error) {
	query := `SELECT ` + intentAnalysisRunColumns + ` FROM monitor_intent_analysis_runs
WHERE id=$1 AND monitor_id=$2 AND draft_id=$3 AND draft_resource_version=$4 AND kind=$5`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanIntentAnalysisRun(executor.QueryRowContext(ctx, query, runID, monitorID, draftID, resourceVersion, kind))
}

func findIntentRunStatus(ctx context.Context, executor intentExecutor, monitorID, runID int64, kind string) (intentAnalysisRunRecord, error) {
	return scanIntentAnalysisRun(executor.QueryRowContext(ctx, `SELECT `+intentAnalysisRunColumns+` FROM monitor_intent_analysis_runs
WHERE id=$1 AND monitor_id=$2 AND kind=$3`, runID, monitorID, kind))
}

func findIntentRunByID(ctx context.Context, executor intentExecutor, runID int64, lock bool) (intentAnalysisRunRecord, error) {
	query := `SELECT ` + intentAnalysisRunColumns + ` FROM monitor_intent_analysis_runs WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanIntentAnalysisRun(executor.QueryRowContext(ctx, query, runID))
}

func findIntentRunByIdempotencyKey(ctx context.Context, executor intentExecutor, key string, lock bool) (intentAnalysisRunRecord, error) {
	query := `SELECT ` + intentAnalysisRunColumns + ` FROM monitor_intent_analysis_runs WHERE idempotency_key=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanIntentAnalysisRun(executor.QueryRowContext(ctx, query, key))
}

func findIntentRunByRiverJob(ctx context.Context, executor intentExecutor, jobID int64, lock bool) (intentAnalysisRunRecord, error) {
	query := `SELECT ` + intentAnalysisRunColumns + ` FROM monitor_intent_analysis_runs WHERE river_job_id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanIntentAnalysisRun(executor.QueryRowContext(ctx, query, jobID))
}

func sameIntentRunIdentityDTO(left, right monitorapplication.IntentRunDTO) bool {
	return left.ID == right.ID && left.Kind == right.Kind && left.MonitorID == right.MonitorID && left.DraftID == right.DraftID &&
		left.DraftResourceVersion == right.DraftResourceVersion && left.InputHash == right.InputHash && left.QueuedAt.Equal(right.QueuedAt)
}

func validIntentSuccessTransition(expected, next monitorapplication.IntentRunDTO, status string) bool {
	return sameIntentRunIdentityDTO(expected, next) && expected.Status == "running" && next.Status == status &&
		next.StartedAt != nil && next.CompletedAt != nil && next.FailureReason == "" &&
		(status != "invalidated" || next.InvalidatedAt != nil)
}

func validIntentWorkerTransition(expected, next monitorapplication.IntentRunDTO) bool {
	switch {
	case expected.Status == "queued" && next.Status == "running":
		return expected.StartedAt == nil && next.StartedAt != nil && next.CompletedAt == nil && next.InvalidatedAt == nil && next.FailureReason == ""
	case (expected.Status == "queued" || expected.Status == "running") && next.Status == "failed":
		return next.CompletedAt != nil && next.InvalidatedAt == nil && next.FailureReason != "" &&
			(expected.Status != "running" || sameIntentOptionalTime(expected.StartedAt, next.StartedAt))
	default:
		return false
	}
}

func updateIntentRunLifecycle(ctx context.Context, executor intentExecutor, next monitorapplication.IntentRunDTO, resultFingerprint string) (intentAnalysisRunRecord, error) {
	return scanIntentAnalysisRun(executor.QueryRowContext(ctx, `
UPDATE monitor_intent_analysis_runs
SET status=$2,started_at=$3,completed_at=$4,invalidated_at=$5,
    failure_reason=NULLIF($6,''),result_fingerprint=NULLIF($7,''),updated_at=CURRENT_TIMESTAMP
WHERE id=$1
RETURNING `+intentAnalysisRunColumns, next.ID, next.Status, next.StartedAt, next.CompletedAt, next.InvalidatedAt,
		next.FailureReason, resultFingerprint))
}

func readIntentRunCandidates(ctx context.Context, executor intentExecutor, runID int64) ([]monitorapplication.ExpansionCandidateDTO, error) {
	rows, err := executor.QueryContext(ctx, `
SELECT id,draft_id,introduced_resource_version,candidate_id,candidate_value,source,reason,
       model_version,prompt_version,input_hash,similarity,risk
FROM monitor_intent_expansion_candidates WHERE origin_run_id=$1 ORDER BY candidate_id,id`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]monitorapplication.ExpansionCandidateDTO, 0)
	for rows.Next() {
		var record intentExpansionCandidateRecord
		if err := rows.Scan(&record.ID, &record.DraftID, &record.IntroducedResourceVersion, &record.CandidateID,
			&record.Value, &record.Source, &record.Reason, &record.ModelVersion, &record.PromptVersion,
			&record.InputHash, &record.Similarity, &record.Risk); err != nil {
			return nil, err
		}
		items = append(items, monitorapplication.ExpansionCandidateDTO{
			ID: record.CandidateID, Value: record.Value, Source: record.Source, Reason: record.Reason,
			ModelVersion: record.ModelVersion, PromptVersion: record.PromptVersion, InputHash: record.InputHash,
			Similarity: record.Similarity, Risk: record.Risk, ApprovalStatus: "pending",
		})
	}
	return items, rows.Err()
}

func insertIntentPreview(ctx context.Context, executor intentExecutor, runID int64, preview monitorapplication.IntentPreviewDTO) error {
	if _, err := executor.ExecContext(ctx, `INSERT INTO monitor_intent_preview_results (run_id,estimated_alert_count) VALUES ($1,$2)`, runID, preview.EstimatedAlertCount); err != nil {
		return err
	}
	for ordinal, sample := range preview.Samples {
		var sampleID int64
		if err := executor.QueryRowContext(ctx, `
INSERT INTO monitor_intent_preview_samples (run_id,ordinal,document_version_id,title,decision)
VALUES ($1,$2,$3,$4,$5) RETURNING id`, runID, ordinal, sample.DocumentVersionID, sample.Title, sample.Decision).Scan(&sampleID); err != nil {
			return err
		}
		for signalOrdinal, signal := range sample.RecallSignals {
			if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_preview_recall_signals (sample_id,run_id,ordinal,channel,rank,score)
VALUES ($1,$2,$3,$4,$5,$6)`, sampleID, runID, signalOrdinal, signal.Channel, signal.Rank, signal.Score); err != nil {
				return err
			}
		}
		for reasonOrdinal, reason := range sample.Reasons {
			if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_preview_reasons (sample_id,run_id,ordinal,reason_type,reason)
VALUES ($1,$2,$3,'match',$4)`, sampleID, runID, reasonOrdinal, reason); err != nil {
				return err
			}
		}
		for reasonOrdinal, reason := range sample.ExclusionReasons {
			if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_preview_reasons (sample_id,run_id,ordinal,reason_type,reason)
VALUES ($1,$2,$3,'exclusion',$4)`, sampleID, runID, reasonOrdinal, reason); err != nil {
				return err
			}
		}
	}
	for ordinal, warning := range preview.Warnings {
		if _, err := executor.ExecContext(ctx, `
INSERT INTO monitor_intent_preview_warnings (run_id,ordinal,warning) VALUES ($1,$2,$3)`, runID, ordinal, warning); err != nil {
			return err
		}
	}
	return nil
}

func readIntentPreview(ctx context.Context, executor intentExecutor, runID int64) (*monitorapplication.IntentPreviewDTO, error) {
	var preview monitorapplication.IntentPreviewDTO
	err := executor.QueryRowContext(ctx, `SELECT estimated_alert_count FROM monitor_intent_preview_results WHERE run_id=$1`, runID).Scan(&preview.EstimatedAlertCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	type sampleIdentity struct {
		ID  int64
		DTO monitorapplication.PreviewSampleDTO
	}
	rows, err := executor.QueryContext(ctx, `
SELECT id,document_version_id,title,decision FROM monitor_intent_preview_samples WHERE run_id=$1 ORDER BY ordinal`, runID)
	if err != nil {
		return nil, err
	}
	samples := make([]sampleIdentity, 0)
	for rows.Next() {
		var sample sampleIdentity
		if err := rows.Scan(&sample.ID, &sample.DTO.DocumentVersionID, &sample.DTO.Title, &sample.DTO.Decision); err != nil {
			_ = rows.Close()
			return nil, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, sample := range samples {
		signals, err := executor.QueryContext(ctx, `
SELECT channel,rank,score FROM monitor_intent_preview_recall_signals WHERE sample_id=$1 ORDER BY ordinal`, sample.ID)
		if err != nil {
			return nil, err
		}
		for signals.Next() {
			var signal monitorapplication.PreviewRecallSignalDTO
			if err := signals.Scan(&signal.Channel, &signal.Rank, &signal.Score); err != nil {
				_ = signals.Close()
				return nil, err
			}
			sample.DTO.RecallSignals = append(sample.DTO.RecallSignals, signal)
		}
		if err := signals.Err(); err != nil {
			_ = signals.Close()
			return nil, err
		}
		if err := signals.Close(); err != nil {
			return nil, err
		}
		reasons, err := executor.QueryContext(ctx, `
SELECT reason_type,reason FROM monitor_intent_preview_reasons WHERE sample_id=$1 ORDER BY reason_type,ordinal`, sample.ID)
		if err != nil {
			return nil, err
		}
		for reasons.Next() {
			var kind, reason string
			if err := reasons.Scan(&kind, &reason); err != nil {
				_ = reasons.Close()
				return nil, err
			}
			if kind == "match" {
				sample.DTO.Reasons = append(sample.DTO.Reasons, reason)
			} else {
				sample.DTO.ExclusionReasons = append(sample.DTO.ExclusionReasons, reason)
			}
		}
		if err := reasons.Err(); err != nil {
			_ = reasons.Close()
			return nil, err
		}
		if err := reasons.Close(); err != nil {
			return nil, err
		}
		preview.Samples = append(preview.Samples, sample.DTO)
	}
	warnings, err := executor.QueryContext(ctx, `SELECT warning FROM monitor_intent_preview_warnings WHERE run_id=$1 ORDER BY ordinal`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = warnings.Close() }()
	for warnings.Next() {
		var warning string
		if err := warnings.Scan(&warning); err != nil {
			return nil, err
		}
		preview.Warnings = append(preview.Warnings, warning)
	}
	return &preview, warnings.Err()
}
