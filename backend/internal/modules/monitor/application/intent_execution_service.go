package application

import (
	"context"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
)

type loadedIntentRun struct {
	run       domain.IntentAnalysisRun
	expansion ExpansionRunDTO
	preview   PreviewRunDTO
}

// StartIntentRun claims one queued run through a lifecycle compare-and-swap.
// A duplicate delivery of the same durable task reuses the existing running
// fact instead of moving its start time.
func (service *IntentService) StartIntentRun(ctx context.Context, command StartIntentRunCommand) (StartIntentRunResult, error) {
	loaded, err := service.loadIntentRun(ctx, command.Run)
	if err != nil {
		return StartIntentRunResult{}, err
	}
	if loaded.run.Status() == domain.IntentRunRunning {
		return StartIntentRunResult{Run: intentRunToDTO(loaded.run), Reused: true}, nil
	}
	if loaded.run.Status() == domain.IntentRunSucceeded || loaded.run.Status() == domain.IntentRunFailed || loaded.run.Status() == domain.IntentRunInvalidated {
		return StartIntentRunResult{Run: intentRunToDTO(loaded.run), Reused: true}, nil
	}
	if loaded.run.Status() != domain.IntentRunQueued {
		return StartIntentRunResult{}, ErrIntentRunStateConflict
	}
	now, err := service.now()
	if err != nil {
		return StartIntentRunResult{}, err
	}
	next, err := loaded.run.Start(now)
	if err != nil {
		return StartIntentRunResult{}, translateIntentDomainError(err)
	}
	persisted, changed, err := service.saveIntentRunTransition(ctx, loaded.run, next)
	if err != nil {
		return StartIntentRunResult{}, err
	}
	if persisted.Status() != domain.IntentRunRunning {
		return StartIntentRunResult{}, invalidIntentContract(fmt.Errorf("run start receipt is not running"))
	}
	return StartIntentRunResult{Run: intentRunToDTO(persisted), Reused: !changed}, nil
}

// FailIntentRun records a terminal worker failure through CAS. The same
// failure is safe to redeliver; a different terminal reason is a conflict.
func (service *IntentService) FailIntentRun(ctx context.Context, command FailIntentRunCommand) (FailIntentRunResult, error) {
	loaded, err := service.loadIntentRun(ctx, command.Run)
	if err != nil {
		return FailIntentRunResult{}, err
	}
	if loaded.run.Status() == domain.IntentRunFailed {
		if loaded.run.FailureReason() != command.Reason {
			return FailIntentRunResult{}, ErrIntentRunResultConflict
		}
		return FailIntentRunResult{Run: intentRunToDTO(loaded.run), Reused: true}, nil
	}
	if loaded.run.Status() != domain.IntentRunQueued && loaded.run.Status() != domain.IntentRunRunning {
		return FailIntentRunResult{}, ErrIntentRunStateConflict
	}
	now, err := service.now()
	if err != nil {
		return FailIntentRunResult{}, err
	}
	next, err := loaded.run.Fail(command.Reason, now)
	if err != nil {
		return FailIntentRunResult{}, translateIntentDomainError(err)
	}
	persisted, changed, err := service.saveIntentRunTransition(ctx, loaded.run, next)
	if err != nil {
		return FailIntentRunResult{}, err
	}
	if persisted.Status() != domain.IntentRunFailed || persisted.FailureReason() != next.FailureReason() {
		return FailIntentRunResult{}, ErrIntentRunResultConflict
	}
	return FailIntentRunResult{Run: intentRunToDTO(persisted), Reused: !changed}, nil
}

// CompleteExpansionRun atomically stores pending candidates and advances the
// exact draft they were generated from. Advancing the draft makes the source
// run historical, so its successful completion is retained with invalidated
// status and remains readable for audit.
func (service *IntentService) CompleteExpansionRun(ctx context.Context, command CompleteExpansionRunCommand) (CompleteExpansionRunResult, error) {
	if command.Run.Kind != string(domain.IntentRunExpansion) {
		return CompleteExpansionRunResult{}, ErrInvalidIntentContract
	}
	loaded, err := service.loadIntentRun(ctx, command.Run)
	if err != nil {
		return CompleteExpansionRunResult{}, err
	}
	candidates, candidateDTOs, err := canonicalPendingExpansionCandidates(command.Candidates, loaded.run.InputHash())
	if err != nil {
		return CompleteExpansionRunResult{}, err
	}
	resultFingerprint := expansionResultFingerprint(candidateDTOs)
	if intentRunHasSuccessfulResult(loaded.run) {
		storedCandidates, err := canonicalExpansionCandidateDTOs(loaded.expansion.Candidates, loaded.run.InputHash(), true)
		if err != nil {
			return CompleteExpansionRunResult{}, err
		}
		if loaded.run.Status() != domain.IntentRunInvalidated || resultFingerprint != expansionResultFingerprint(storedCandidates) {
			return CompleteExpansionRunResult{}, ErrIntentRunResultConflict
		}
		return CompleteExpansionRunResult{
			Expansion: cloneExpansionRunDTO(ExpansionRunDTO{Run: intentRunToDTO(loaded.run), Candidates: storedCandidates}),
			Reused:    true,
		}, nil
	}
	if loaded.run.Status() != domain.IntentRunRunning {
		return CompleteExpansionRunResult{}, ErrIntentRunStateConflict
	}
	now, err := service.now()
	if err != nil {
		return CompleteExpansionRunResult{}, err
	}
	succeeded, err := loaded.run.Succeed(now)
	if err != nil {
		return CompleteExpansionRunResult{}, translateIntentDomainError(err)
	}
	currentDraft, err := service.loadDraft(ctx, loaded.run.MonitorID(), loaded.run.DraftID())
	if err != nil {
		return CompleteExpansionRunResult{}, err
	}
	nextDraft, err := currentDraft.AttachExpansionCandidates(currentDraft.ResourceVersion(), succeeded, candidates)
	if err != nil {
		return CompleteExpansionRunResult{}, translateIntentDomainError(err)
	}
	invalidated, changed, err := succeeded.InvalidateForDraft(nextDraft.DraftID(), nextDraft.ResourceVersion(), now)
	if err != nil || !changed {
		if err == nil {
			err = domain.ErrIntentRunTransition
		}
		return CompleteExpansionRunResult{}, translateIntentDomainError(err)
	}
	mutation := CompleteExpansionRunMutationDTO{
		Transition: IntentRunTransitionDTO{Expected: intentRunToDTO(loaded.run), Next: intentRunToDTO(invalidated)},
		DraftMutation: IntentDraftMutationDTO{
			Kind: IntentDraftMutationExpansionResult, ExpectedDraftID: currentDraft.DraftID(),
			ExpectedResourceVersion: currentDraft.ResourceVersion(), Next: intentDraftToDTO(nextDraft), InvalidatedAt: now,
		},
		Candidates: candidateDTOs, ResultFingerprint: resultFingerprint,
	}
	receipt, err := service.runs.CompleteExpansion(ctx, mutation)
	if err != nil {
		return CompleteExpansionRunResult{}, err
	}
	canonical, err := validateExpansionCompletionReceipt(receipt, invalidated, nextDraft, resultFingerprint)
	if err != nil {
		return CompleteExpansionRunResult{}, err
	}
	return CompleteExpansionRunResult{Expansion: canonical, Reused: !receipt.Changed}, nil
}

// CompletePreviewRun stores one explainable preview with the success CAS. A
// success row without a result can therefore never become externally visible.
func (service *IntentService) CompletePreviewRun(ctx context.Context, command CompletePreviewRunCommand) (CompletePreviewRunResult, error) {
	if command.Run.Kind != string(domain.IntentRunPreview) {
		return CompletePreviewRunResult{}, ErrInvalidIntentContract
	}
	loaded, err := service.loadIntentRun(ctx, command.Run)
	if err != nil {
		return CompletePreviewRunResult{}, err
	}
	preview := clonePreviewRunDTO(PreviewRunDTO{Preview: &command.Preview}).Preview
	if err := validatePreviewDTO(preview); err != nil {
		return CompletePreviewRunResult{}, err
	}
	resultFingerprint := previewResultFingerprint(*preview)
	if intentRunHasSuccessfulResult(loaded.run) {
		if loaded.preview.Preview == nil {
			return CompletePreviewRunResult{}, invalidIntentContract(fmt.Errorf("successful preview run has no result"))
		}
		stored := clonePreviewRunDTO(loaded.preview)
		if resultFingerprint != previewResultFingerprint(*stored.Preview) {
			return CompletePreviewRunResult{}, ErrIntentRunResultConflict
		}
		return CompletePreviewRunResult{Preview: stored, Reused: true}, nil
	}
	if loaded.run.Status() != domain.IntentRunRunning {
		return CompletePreviewRunResult{}, ErrIntentRunStateConflict
	}
	now, err := service.now()
	if err != nil {
		return CompletePreviewRunResult{}, err
	}
	succeeded, err := loaded.run.Succeed(now)
	if err != nil {
		return CompletePreviewRunResult{}, translateIntentDomainError(err)
	}
	receipt, err := service.runs.CompletePreview(ctx, CompletePreviewRunMutationDTO{
		Transition: IntentRunTransitionDTO{Expected: intentRunToDTO(loaded.run), Next: intentRunToDTO(succeeded)},
		Preview:    *preview, ResultFingerprint: resultFingerprint,
	})
	if err != nil {
		return CompletePreviewRunResult{}, err
	}
	canonical, err := validatePreviewCompletionReceipt(receipt, succeeded, resultFingerprint)
	if err != nil {
		return CompletePreviewRunResult{}, err
	}
	return CompletePreviewRunResult{Preview: canonical, Reused: !receipt.Changed}, nil
}

func (service *IntentService) loadIntentRun(ctx context.Context, reference IntentRunReferenceDTO) (loadedIntentRun, error) {
	kind := domain.IntentRunKind(reference.Kind)
	if validateIntentRunQuery(reference.MonitorID, reference.DraftID, reference.DraftResourceVersion, reference.RunID) != nil ||
		!kind.Valid() || !validIntentApplicationSHA256(reference.InputHash) {
		return loadedIntentRun{}, ErrInvalidIntentContract
	}
	switch kind {
	case domain.IntentRunExpansion:
		stored, err := service.runs.FindExpansion(ctx, ReadExpansionRunQuery{
			MonitorID: reference.MonitorID, DraftID: reference.DraftID,
			DraftResourceVersion: reference.DraftResourceVersion, RunID: reference.RunID,
		})
		if err != nil {
			return loadedIntentRun{}, err
		}
		run, err := validateStoredIntentRun(stored.Run, reference.MonitorID, reference.DraftID, reference.DraftResourceVersion, reference.RunID, kind)
		if err != nil {
			return loadedIntentRun{}, err
		}
		if run.InputHash() != reference.InputHash {
			return loadedIntentRun{}, invalidIntentContract(fmt.Errorf("durable task input hash does not match its run"))
		}
		if err := validateIntentRunResultVisibility(run, len(stored.Candidates) != 0, false); err != nil {
			return loadedIntentRun{}, err
		}
		return loadedIntentRun{run: run, expansion: cloneExpansionRunDTO(stored)}, nil
	case domain.IntentRunPreview:
		stored, err := service.runs.FindPreview(ctx, ReadPreviewRunQuery{
			MonitorID: reference.MonitorID, DraftID: reference.DraftID,
			DraftResourceVersion: reference.DraftResourceVersion, RunID: reference.RunID,
		})
		if err != nil {
			return loadedIntentRun{}, err
		}
		run, err := validateStoredIntentRun(stored.Run, reference.MonitorID, reference.DraftID, reference.DraftResourceVersion, reference.RunID, kind)
		if err != nil {
			return loadedIntentRun{}, err
		}
		if run.InputHash() != reference.InputHash {
			return loadedIntentRun{}, invalidIntentContract(fmt.Errorf("durable task input hash does not match its run"))
		}
		if err := validateIntentRunResultVisibility(run, stored.Preview != nil, true); err != nil {
			return loadedIntentRun{}, err
		}
		if stored.Preview != nil {
			if err := validatePreviewDTO(stored.Preview); err != nil {
				return loadedIntentRun{}, err
			}
		}
		return loadedIntentRun{run: run, preview: clonePreviewRunDTO(stored)}, nil
	default:
		return loadedIntentRun{}, ErrInvalidIntentContract
	}
}

func (service *IntentService) saveIntentRunTransition(ctx context.Context, expected, next domain.IntentAnalysisRun) (domain.IntentAnalysisRun, bool, error) {
	receipt, err := service.runs.SaveTransition(ctx, IntentRunTransitionDTO{Expected: intentRunToDTO(expected), Next: intentRunToDTO(next)})
	if err != nil {
		return domain.IntentAnalysisRun{}, false, err
	}
	persisted, err := intentRunFromDTO(receipt.Run)
	if err != nil {
		return domain.IntentAnalysisRun{}, false, err
	}
	if !sameIntentRunIdentity(persisted, next) {
		return domain.IntentAnalysisRun{}, false, invalidIntentContract(fmt.Errorf("run transition receipt belongs to another run"))
	}
	if receipt.Changed && !reflect.DeepEqual(intentRunToDTO(persisted), intentRunToDTO(next)) {
		return domain.IntentAnalysisRun{}, false, invalidIntentContract(fmt.Errorf("repository changed run transition facts"))
	}
	return persisted, receipt.Changed, nil
}

func validateExpansionCompletionReceipt(receipt CompleteExpansionRunReceiptDTO, expectedRun domain.IntentAnalysisRun, expectedDraft domain.IntentDraft, fingerprint string) (ExpansionRunDTO, error) {
	if receipt.ResultFingerprint != fingerprint {
		return ExpansionRunDTO{}, ErrIntentRunResultConflict
	}
	run, err := intentRunFromDTO(receipt.Expansion.Run)
	if err != nil {
		return ExpansionRunDTO{}, err
	}
	if !sameIntentRunIdentity(run, expectedRun) || run.Status() != domain.IntentRunInvalidated || !intentRunHasSuccessfulResult(run) {
		return ExpansionRunDTO{}, invalidIntentContract(fmt.Errorf("expansion completion receipt has an invalid run"))
	}
	candidates, err := canonicalExpansionCandidateDTOs(receipt.Expansion.Candidates, run.InputHash(), true)
	if err != nil {
		return ExpansionRunDTO{}, err
	}
	if expansionResultFingerprint(candidates) != fingerprint {
		return ExpansionRunDTO{}, ErrIntentRunResultConflict
	}
	draft, err := intentDraftFromDTO(receipt.Draft)
	if err != nil {
		return ExpansionRunDTO{}, err
	}
	if !reflect.DeepEqual(intentDraftToDTO(draft), intentDraftToDTO(expectedDraft)) {
		return ExpansionRunDTO{}, invalidIntentContract(fmt.Errorf("repository changed expansion draft facts"))
	}
	if receipt.Changed && !reflect.DeepEqual(intentRunToDTO(run), intentRunToDTO(expectedRun)) {
		return ExpansionRunDTO{}, invalidIntentContract(fmt.Errorf("repository changed expansion completion facts"))
	}
	return cloneExpansionRunDTO(ExpansionRunDTO{Run: intentRunToDTO(run), Candidates: candidates}), nil
}

func validatePreviewCompletionReceipt(receipt CompletePreviewRunReceiptDTO, expectedRun domain.IntentAnalysisRun, fingerprint string) (PreviewRunDTO, error) {
	if receipt.ResultFingerprint != fingerprint || receipt.Preview.Preview == nil {
		return PreviewRunDTO{}, ErrIntentRunResultConflict
	}
	run, err := intentRunFromDTO(receipt.Preview.Run)
	if err != nil {
		return PreviewRunDTO{}, err
	}
	if !sameIntentRunIdentity(run, expectedRun) || !intentRunHasSuccessfulResult(run) {
		return PreviewRunDTO{}, invalidIntentContract(fmt.Errorf("preview completion receipt has an invalid run"))
	}
	if err := validatePreviewDTO(receipt.Preview.Preview); err != nil {
		return PreviewRunDTO{}, err
	}
	if previewResultFingerprint(*receipt.Preview.Preview) != fingerprint {
		return PreviewRunDTO{}, ErrIntentRunResultConflict
	}
	if receipt.Changed && !reflect.DeepEqual(intentRunToDTO(run), intentRunToDTO(expectedRun)) {
		return PreviewRunDTO{}, invalidIntentContract(fmt.Errorf("repository changed preview completion facts"))
	}
	return clonePreviewRunDTO(receipt.Preview), nil
}

func canonicalPendingExpansionCandidates(items []ExpansionCandidateDTO, inputHash string) ([]domain.ExpansionCandidate, []ExpansionCandidateDTO, error) {
	canonical, err := canonicalExpansionCandidateDTOs(items, inputHash, true)
	if err != nil {
		return nil, nil, err
	}
	candidates := make([]domain.ExpansionCandidate, 0, len(canonical))
	for _, item := range canonical {
		candidate, candidateErr := expansionCandidateFromDTO(item)
		if candidateErr != nil {
			return nil, nil, candidateErr
		}
		candidates = append(candidates, candidate)
	}
	return candidates, canonical, nil
}

func canonicalExpansionCandidateDTOs(items []ExpansionCandidateDTO, inputHash string, pendingOnly bool) ([]ExpansionCandidateDTO, error) {
	canonical := make([]ExpansionCandidateDTO, 0, len(items))
	for _, item := range items {
		candidate, err := expansionCandidateFromDTO(item)
		if err != nil {
			return nil, err
		}
		if candidate.Provenance().InputHash() != inputHash || pendingOnly && candidate.ApprovalStatus() != domain.ExpansionApprovalPending {
			return nil, invalidIntentContract(fmt.Errorf("expansion result candidate does not belong to the run"))
		}
		canonical = append(canonical, expansionCandidateToDTO(candidate))
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ID < canonical[j].ID })
	for index := 1; index < len(canonical); index++ {
		if canonical[index-1].ID == canonical[index].ID {
			return nil, invalidIntentContract(fmt.Errorf("expansion result has duplicate candidate ids"))
		}
	}
	return canonical, nil
}

func expansionResultFingerprint(candidates []ExpansionCandidateDTO) string {
	parts := []string{"expansion-result-v1", strconv.Itoa(len(candidates))}
	for _, candidate := range candidates {
		parts = append(parts,
			candidate.ID, candidate.Value, candidate.Source, candidate.Reason,
			candidate.ModelVersion, candidate.PromptVersion, candidate.InputHash,
			strconv.FormatFloat(candidate.Similarity, 'g', -1, 64), candidate.Risk, candidate.ApprovalStatus,
		)
	}
	return intentRunHash(parts...)
}

func previewResultFingerprint(preview IntentPreviewDTO) string {
	parts := []string{"preview-result-v1", strconv.Itoa(preview.EstimatedAlertCount), strconv.Itoa(len(preview.Warnings))}
	parts = append(parts, preview.Warnings...)
	parts = append(parts, strconv.Itoa(len(preview.Samples)))
	for _, sample := range preview.Samples {
		parts = append(parts, strconv.FormatInt(sample.DocumentVersionID, 10), sample.Title, sample.Decision)
		parts = append(parts, strconv.Itoa(len(sample.RecallSignals)))
		for _, signal := range sample.RecallSignals {
			parts = append(parts, signal.Channel, strconv.Itoa(signal.Rank), strconv.FormatFloat(signal.Score, 'g', -1, 64))
		}
		parts = append(parts, strconv.Itoa(len(sample.Reasons)))
		parts = append(parts, sample.Reasons...)
		parts = append(parts, strconv.Itoa(len(sample.ExclusionReasons)))
		parts = append(parts, sample.ExclusionReasons...)
	}
	return intentRunHash(parts...)
}

func sameIntentRunIdentity(left, right domain.IntentAnalysisRun) bool {
	return left.ID() == right.ID() && left.Kind() == right.Kind() && left.MonitorID() == right.MonitorID() &&
		left.DraftID() == right.DraftID() && left.DraftResourceVersion() == right.DraftResourceVersion() && left.InputHash() == right.InputHash()
}

func validIntentApplicationSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func intentRunHasSuccessfulResult(run domain.IntentAnalysisRun) bool {
	if run.Status() == domain.IntentRunSucceeded {
		return true
	}
	return run.Status() == domain.IntentRunInvalidated && run.StartedAt() != nil && run.CompletedAt() != nil && run.FailureReason() == ""
}

func validateIntentRunResultVisibility(run domain.IntentAnalysisRun, hasResult, resultRequired bool) error {
	canExpose := intentRunHasSuccessfulResult(run)
	if hasResult && !canExpose {
		return invalidIntentContract(fmt.Errorf("unfinished intent run contains a result"))
	}
	if resultRequired && canExpose && !hasResult {
		return invalidIntentContract(fmt.Errorf("successful intent run has no result"))
	}
	return nil
}
