package application

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
)

// ReadIntentAnalysisTask expands the three-field River identity into durable
// run facts. Callers cannot supply a kind, Monitor ID, input hash, profile, or
// preview limit through the queue payload.
func (service *IntentService) ReadIntentAnalysisTask(ctx context.Context, query ReadIntentAnalysisTaskQuery) (ReadIntentAnalysisTaskResult, error) {
	if service == nil || service.tasks == nil || query.RunID <= 0 || query.DraftID <= 0 || query.DraftResourceVersion <= 0 {
		return ReadIntentAnalysisTaskResult{}, ErrInvalidIntentContract
	}
	task, err := service.tasks.FindIntentAnalysisTask(ctx, query)
	if err != nil {
		return ReadIntentAnalysisTaskResult{}, err
	}
	if err := validateIntentAnalysisTask(task, query); err != nil {
		return ReadIntentAnalysisTaskResult{}, err
	}
	return ReadIntentAnalysisTaskResult{Task: task}, nil
}

// PrepareIntentExpansion independently rereads both the run and its exact
// append-only draft revision. A processor therefore cannot reuse a task DTO
// from another run or silently switch to the latest draft revision.
func (service *IntentService) PrepareIntentExpansion(ctx context.Context, query PrepareIntentExpansionQuery) (PrepareIntentExpansionResult, error) {
	if service == nil || service.tasks == nil || service.revisions == nil {
		return PrepareIntentExpansionResult{}, ErrInvalidIntentContract
	}
	run := query.Task.Run
	resolved, err := service.ReadIntentAnalysisTask(ctx, ReadIntentAnalysisTaskQuery{
		RunID: run.RunID, DraftID: run.DraftID, DraftResourceVersion: run.DraftResourceVersion,
	})
	if err != nil {
		return PrepareIntentExpansionResult{}, err
	}
	if !reflect.DeepEqual(resolved.Task, query.Task) || run.Kind != string(domain.IntentRunExpansion) || query.Task.SampleLimit != 0 {
		return PrepareIntentExpansionResult{}, invalidIntentContract(fmt.Errorf("expansion task differs from its durable run"))
	}
	stored, err := service.revisions.FindIntentDraftRevision(ctx, ReadIntentDraftRevisionQuery{
		MonitorID: run.MonitorID, DraftID: run.DraftID, ResourceVersion: run.DraftResourceVersion,
	})
	if err != nil {
		return PrepareIntentExpansionResult{}, err
	}
	draft, err := intentDraftFromDTO(stored)
	if err != nil {
		return PrepareIntentExpansionResult{}, err
	}
	if draft.MonitorID() != run.MonitorID || draft.DraftID() != run.DraftID || draft.ResourceVersion() != run.DraftResourceVersion {
		return PrepareIntentExpansionResult{}, invalidIntentContract(fmt.Errorf("immutable draft revision identity drifted"))
	}
	if intentExpansionInputHash(draft, query.Task.AnalysisProfile) != run.InputHash {
		return PrepareIntentExpansionResult{}, invalidIntentContract(fmt.Errorf("expansion input hash does not match the immutable draft revision"))
	}
	return PrepareIntentExpansionResult{Expansion: PreparedIntentExpansionDTO{
		Task: resolved.Task, Draft: intentDraftToDTO(draft),
	}}, nil
}

// PrepareIntentPreview independently reloads the durable run and its exact
// append-only draft revision, then recomputes the input hash. This is the only
// draft projection accepted by the compiler/evaluator path.
func (service *IntentService) PrepareIntentPreview(ctx context.Context, query PrepareIntentPreviewQuery) (PrepareIntentPreviewResult, error) {
	if service == nil || service.tasks == nil || service.revisions == nil {
		return PrepareIntentPreviewResult{}, ErrInvalidIntentContract
	}
	run := query.Task.Run
	resolved, err := service.ReadIntentAnalysisTask(ctx, ReadIntentAnalysisTaskQuery{
		RunID: run.RunID, DraftID: run.DraftID, DraftResourceVersion: run.DraftResourceVersion,
	})
	if err != nil {
		return PrepareIntentPreviewResult{}, err
	}
	if !reflect.DeepEqual(resolved.Task, query.Task) || run.Kind != string(domain.IntentRunPreview) || query.Task.SampleLimit < 1 || query.Task.SampleLimit > 200 {
		return PrepareIntentPreviewResult{}, invalidIntentContract(fmt.Errorf("preview task differs from its durable run"))
	}
	stored, err := service.revisions.FindIntentDraftRevision(ctx, ReadIntentDraftRevisionQuery{
		MonitorID: run.MonitorID, DraftID: run.DraftID, ResourceVersion: run.DraftResourceVersion,
	})
	if err != nil {
		return PrepareIntentPreviewResult{}, err
	}
	draft, err := intentDraftFromDTO(stored)
	if err != nil {
		return PrepareIntentPreviewResult{}, err
	}
	if draft.MonitorID() != run.MonitorID || draft.DraftID() != run.DraftID || draft.ResourceVersion() != run.DraftResourceVersion {
		return PrepareIntentPreviewResult{}, invalidIntentContract(fmt.Errorf("immutable draft revision identity drifted"))
	}
	if intentPreviewInputHash(draft, query.Task.AnalysisProfile, query.Task.SampleLimit) != run.InputHash {
		return PrepareIntentPreviewResult{}, invalidIntentContract(fmt.Errorf("preview input hash does not match the immutable draft revision"))
	}
	return PrepareIntentPreviewResult{Preview: PreparedIntentPreviewDTO{
		Task: resolved.Task, Draft: intentDraftToDTO(draft),
	}}, nil
}

func validateIntentAnalysisTask(task IntentAnalysisTaskDTO, query ReadIntentAnalysisTaskQuery) error {
	run := task.Run
	kind := domain.IntentRunKind(run.Kind)
	if run.RunID != query.RunID || run.DraftID != query.DraftID || run.DraftResourceVersion != query.DraftResourceVersion ||
		run.MonitorID <= 0 || !kind.Valid() || !validIntentApplicationSHA256(run.InputHash) {
		return invalidIntentContract(fmt.Errorf("durable intent analysis task identity is invalid"))
	}
	switch kind {
	case domain.IntentRunExpansion:
		if _, err := validateIntentExpansionProfile(task.AnalysisProfile); err != nil || task.SampleLimit != 0 {
			return invalidIntentContract(fmt.Errorf("expansion task contains preview parameters"))
		}
	case domain.IntentRunPreview:
		if _, err := validateIntentProfile(task.AnalysisProfile); err != nil || task.SampleLimit < 1 || task.SampleLimit > 200 {
			return invalidIntentContract(fmt.Errorf("preview task sample limit is invalid"))
		}
	}
	return nil
}

func intentExpansionInputHash(draft domain.IntentDraft, profile string) string {
	return intentRunHash(
		"expansion-input-v1", strconv.FormatInt(draft.DraftID(), 10),
		strconv.FormatInt(draft.ResourceVersion(), 10), draft.MatchingFingerprint(), profile, "0",
	)
}
