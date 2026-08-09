package application

import (
	"context"
	"errors"
	stdhttp "net/http"

	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// IntentControlService is the monitor-scoped use-case boundary used by HTTP.
// The core IntentService remains exact-draft based for workers and persistence;
// this facade resolves the current configuration draft and repeats RBAC.
type IntentControlService struct {
	intents       *IntentService
	currentDrafts CurrentIntentDraftRepository
	runStatuses   IntentRunStatusRepository
	analysis      IntentAnalysisAvailability
	authorizer    IntentControlAuthorizer
}

func NewIntentControlService(dependencies IntentControlDependencies) (*IntentControlService, error) {
	if dependencies.Intents == nil || dependencies.CurrentDrafts == nil || dependencies.RunStatuses == nil || dependencies.Authorizer == nil {
		return nil, ErrInvalidIntentContract
	}
	return &IntentControlService{
		intents: dependencies.Intents, currentDrafts: dependencies.CurrentDrafts,
		runStatuses: dependencies.RunStatuses, analysis: dependencies.Analysis, authorizer: dependencies.Authorizer,
	}, nil
}

func (service *IntentControlService) ReadDraft(ctx context.Context, query ReadCurrentIntentDraftQuery) (ReadCurrentIntentDraftResult, error) {
	if err := service.authorize(ctx, query.Actor, query.MonitorID, IntentControlReadDraft); err != nil {
		return ReadCurrentIntentDraftResult{}, err
	}
	draft, err := service.currentDrafts.FindCurrent(ctx, ReadCurrentIntentDraftRepositoryQuery{MonitorID: query.MonitorID})
	if err != nil {
		return ReadCurrentIntentDraftResult{}, intentControlError(err)
	}
	if err := validateCurrentIntentDraft(draft, query.MonitorID); err != nil {
		return ReadCurrentIntentDraftResult{}, err
	}
	return ReadCurrentIntentDraftResult{Draft: cloneIntentDraftDTO(draft)}, nil
}

func (service *IntentControlService) PutDraft(ctx context.Context, command PutCurrentIntentDraftCommand) (PutCurrentIntentDraftResult, error) {
	if err := service.authorize(ctx, command.Actor, command.MonitorID, IntentControlReplaceDraft); err != nil {
		return PutCurrentIntentDraftResult{}, err
	}
	if command.MonitorID <= 0 || command.ExpectedResourceVersion < 0 {
		return PutCurrentIntentDraftResult{}, intentControlError(ErrInvalidIntentContract)
	}
	if command.ExpectedResourceVersion == 0 {
		initial := IntentDraftDTO{
			MonitorID: command.MonitorID, ResourceVersion: 1, Objective: command.Objective,
			Clauses: command.Clauses, Entities: command.Entities, Examples: command.Examples,
		}
		// Reuse Domain construction through the core mapper before persistence,
		// while allowing the repository to allocate the durable DraftID.
		definition, err := intentDefinitionFromDTO(initial.Objective, initial.Clauses, initial.Entities, initial.Examples)
		if err != nil {
			return PutCurrentIntentDraftResult{}, intentControlError(err)
		}
		initial.Objective, initial.Clauses, initial.Entities, initial.Examples = intentDefinitionToDTO(definition)
		stored, err := service.currentDrafts.InitializeCurrent(ctx, InitializeCurrentIntentDraftMutationDTO{Initial: initial})
		if err != nil {
			return PutCurrentIntentDraftResult{}, intentControlError(err)
		}
		if err := validateCurrentIntentDraft(stored, command.MonitorID); err != nil || stored.ResourceVersion != 1 {
			if err == nil {
				err = ErrInvalidIntentContract
			}
			return PutCurrentIntentDraftResult{}, intentControlError(err)
		}
		return PutCurrentIntentDraftResult{Draft: cloneIntentDraftDTO(stored), Created: true}, nil
	}

	current, err := service.currentDrafts.FindCurrent(ctx, ReadCurrentIntentDraftRepositoryQuery{MonitorID: command.MonitorID})
	if err != nil {
		return PutCurrentIntentDraftResult{}, intentControlError(err)
	}
	if err := validateCurrentIntentDraft(current, command.MonitorID); err != nil {
		return PutCurrentIntentDraftResult{}, err
	}
	result, err := service.intents.ReplaceDraft(ctx, ReplaceIntentDraftCommand{
		MonitorID: command.MonitorID, DraftID: current.DraftID,
		ExpectedResourceVersion: command.ExpectedResourceVersion,
		Objective:               command.Objective, Clauses: command.Clauses, Entities: command.Entities, Examples: command.Examples,
	})
	if err != nil {
		return PutCurrentIntentDraftResult{}, intentControlError(err)
	}
	return PutCurrentIntentDraftResult{Draft: result.Draft}, nil
}

func (service *IntentControlService) ReviewExpansionCandidate(ctx context.Context, command ReviewCurrentExpansionCandidateCommand) (ReviewExpansionCandidateResult, error) {
	if err := service.authorize(ctx, command.Actor, command.MonitorID, IntentControlReviewCandidate); err != nil {
		return ReviewExpansionCandidateResult{}, err
	}
	draft, err := service.currentDraft(ctx, command.MonitorID)
	if err != nil {
		return ReviewExpansionCandidateResult{}, err
	}
	result, err := service.intents.ReviewCandidate(ctx, ReviewExpansionCandidateCommand{
		MonitorID: command.MonitorID, DraftID: draft.DraftID, CandidateID: command.CandidateID,
		ExpectedResourceVersion: command.ExpectedResourceVersion, Decision: command.Decision,
		ReviewerUserID: command.Actor.UserID, Note: command.Note, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return ReviewExpansionCandidateResult{}, intentControlError(err)
	}
	return result, nil
}

func (service *IntentControlService) SubmitExpansionRun(ctx context.Context, command SubmitCurrentExpansionRunCommand) (SubmitExpansionRunResult, error) {
	if err := service.authorize(ctx, command.Actor, command.MonitorID, IntentControlSubmitExpansion); err != nil {
		return SubmitExpansionRunResult{}, err
	}
	if !service.analysisAvailable("expansion") {
		return SubmitExpansionRunResult{}, sharederrors.New(sharederrors.CodeAIModelUnavailable, stdhttp.StatusServiceUnavailable, "monitor intent expansion is unavailable")
	}
	draft, err := service.currentDraft(ctx, command.MonitorID)
	if err != nil {
		return SubmitExpansionRunResult{}, err
	}
	result, err := service.intents.SubmitExpansionRun(ctx, SubmitExpansionRunCommand{
		MonitorID: command.MonitorID, DraftID: draft.DraftID, ExpectedResourceVersion: command.ExpectedResourceVersion,
		IdempotencyKey: command.IdempotencyKey, ExpansionProfile: command.ExpansionProfile,
	})
	if err != nil {
		return SubmitExpansionRunResult{}, intentControlError(err)
	}
	return result, nil
}

func (service *IntentControlService) SubmitPreviewRun(ctx context.Context, command SubmitCurrentPreviewRunCommand) (SubmitPreviewRunResult, error) {
	if err := service.authorize(ctx, command.Actor, command.MonitorID, IntentControlSubmitPreview); err != nil {
		return SubmitPreviewRunResult{}, err
	}
	if !service.analysisAvailable("preview") {
		return SubmitPreviewRunResult{}, sharederrors.New(sharederrors.CodeAIModelUnavailable, stdhttp.StatusServiceUnavailable, "monitor intent preview is unavailable")
	}
	draft, err := service.currentDraft(ctx, command.MonitorID)
	if err != nil {
		return SubmitPreviewRunResult{}, err
	}
	result, err := service.intents.SubmitPreviewRun(ctx, SubmitPreviewRunCommand{
		MonitorID: command.MonitorID, DraftID: draft.DraftID, ExpectedResourceVersion: command.ExpectedResourceVersion,
		IdempotencyKey: command.IdempotencyKey, EvaluatorProfile: command.EvaluatorProfile, SampleLimit: command.SampleLimit,
	})
	if err != nil {
		return SubmitPreviewRunResult{}, intentControlError(err)
	}
	return result, nil
}

func (service *IntentControlService) ReadExpansionRun(ctx context.Context, query ReadIntentExpansionRunQuery) (ReadExpansionRunResult, error) {
	if err := service.authorize(ctx, query.Actor, query.MonitorID, IntentControlReadExpansion); err != nil {
		return ReadExpansionRunResult{}, err
	}
	stored, err := service.runStatuses.FindExpansionStatus(ctx, IntentRunStatusLookupDTO{MonitorID: query.MonitorID, RunID: query.RunID})
	if err != nil {
		return ReadExpansionRunResult{}, intentControlError(err)
	}
	if stored.Run.MonitorID != query.MonitorID || stored.Run.ID != query.RunID {
		return ReadExpansionRunResult{}, ErrInvalidIntentContract
	}
	result, err := service.intents.ReadExpansionRun(ctx, ReadExpansionRunQuery{
		MonitorID: query.MonitorID, DraftID: stored.Run.DraftID,
		DraftResourceVersion: stored.Run.DraftResourceVersion, RunID: query.RunID,
	})
	if err != nil {
		return ReadExpansionRunResult{}, intentControlError(err)
	}
	return result, nil
}

func (service *IntentControlService) ReadPreviewRun(ctx context.Context, query ReadIntentPreviewRunQuery) (ReadPreviewRunResult, error) {
	if err := service.authorize(ctx, query.Actor, query.MonitorID, IntentControlReadPreview); err != nil {
		return ReadPreviewRunResult{}, err
	}
	stored, err := service.runStatuses.FindPreviewStatus(ctx, IntentRunStatusLookupDTO{MonitorID: query.MonitorID, RunID: query.RunID})
	if err != nil {
		return ReadPreviewRunResult{}, intentControlError(err)
	}
	if stored.Run.MonitorID != query.MonitorID || stored.Run.ID != query.RunID {
		return ReadPreviewRunResult{}, ErrInvalidIntentContract
	}
	result, err := service.intents.ReadPreviewRun(ctx, ReadPreviewRunQuery{
		MonitorID: query.MonitorID, DraftID: stored.Run.DraftID,
		DraftResourceVersion: stored.Run.DraftResourceVersion, RunID: query.RunID,
	})
	if err != nil {
		return ReadPreviewRunResult{}, intentControlError(err)
	}
	return result, nil
}

func (service *IntentControlService) currentDraft(ctx context.Context, monitorID int64) (IntentDraftDTO, error) {
	draft, err := service.currentDrafts.FindCurrent(ctx, ReadCurrentIntentDraftRepositoryQuery{MonitorID: monitorID})
	if err != nil {
		return IntentDraftDTO{}, intentControlError(err)
	}
	if err := validateCurrentIntentDraft(draft, monitorID); err != nil {
		return IntentDraftDTO{}, err
	}
	return draft, nil
}

func (service *IntentControlService) analysisAvailable(kind string) bool {
	return service != nil && service.analysis != nil && service.analysis.Available(kind)
}

func validateCurrentIntentDraft(draft IntentDraftDTO, monitorID int64) error {
	if monitorID <= 0 || draft.MonitorID != monitorID || draft.DraftID <= 0 || draft.ResourceVersion <= 0 {
		return ErrInvalidIntentContract
	}
	return nil
}

func (service *IntentControlService) authorize(ctx context.Context, actor IntentActorDTO, monitorID int64, operation IntentControlOperation) error {
	if service == nil || service.authorizer == nil || actor.UserID <= 0 || monitorID <= 0 {
		return sharederrors.New(sharederrors.CodeForbidden, stdhttp.StatusForbidden, "")
	}
	if err := service.authorizer.AuthorizeIntentControl(ctx, AuthorizeIntentControlQueryDTO{
		ActorUserID: actor.UserID, MonitorID: monitorID, Operation: operation,
	}); err != nil {
		return intentControlError(err)
	}
	return nil
}

func intentControlError(err error) error {
	if err == nil {
		return nil
	}
	var app *sharederrors.AppError
	if errors.As(err, &app) {
		return app
	}
	switch {
	case errors.Is(err, ErrInvalidIntentContract):
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", err)
	case errors.Is(err, ErrIntentDraftNotFound):
		return sharederrors.Wrap(sharederrors.CodeMonitorIntentDraftUninitialized, stdhttp.StatusNotFound, "", err)
	case errors.Is(err, ErrIntentRunNotFound):
		return sharederrors.Wrap(sharederrors.CodeMonitorIntentRunNotFound, stdhttp.StatusNotFound, "", err)
	case errors.Is(err, ErrIntentVersionConflict):
		return sharederrors.Wrap(sharederrors.CodeMonitorIntentVersionConflict, stdhttp.StatusConflict, "", err)
	case errors.Is(err, ErrIntentIdempotencyConflict):
		return sharederrors.Wrap(sharederrors.CodeMonitorIntentIdempotencyConflict, stdhttp.StatusConflict, "", err)
	case errors.Is(err, ErrExpansionCandidateNotFound):
		return sharederrors.Wrap(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "", err)
	case errors.Is(err, ErrExpansionDecisionConflict), errors.Is(err, ErrIntentRunStateConflict), errors.Is(err, ErrIntentRunResultConflict):
		return sharederrors.Wrap(sharederrors.CodeConflict, stdhttp.StatusConflict, "", err)
	case errors.Is(err, ErrIntentAuthorizationDenied):
		return sharederrors.Wrap(sharederrors.CodeForbidden, stdhttp.StatusForbidden, "", err)
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "", err)
	case errors.Is(err, sharedrepository.ErrConflict), errors.Is(err, sharedrepository.ErrConstraint):
		return sharederrors.Wrap(sharederrors.CodeConflict, stdhttp.StatusConflict, "", err)
	default:
		return err
	}
}
