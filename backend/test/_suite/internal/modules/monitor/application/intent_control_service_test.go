package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
)

func TestIntentControlInitializesIndependentCurrentDraftThenUsesResourceCAS(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 17, 0, 0, 0, time.UTC)
	repository := &intentControlRepositoryFake{intentDraftRepositoryFake: intentDraftRepositoryFake{}}
	authorizer := &intentControlAuthorizerFake{roles: map[int64]string{17: "editor"}}
	intents, err := NewIntentService(IntentServiceDependencies{Drafts: repository, Runs: newIntentRunRepositoryFake(now), Clock: fixedIntentClock{now: now}})
	if err != nil {
		t.Fatal(err)
	}
	control, err := NewIntentControlService(IntentControlDependencies{Intents: intents, CurrentDrafts: repository, RunStatuses: repository, Authorizer: authorizer})
	if err != nil {
		t.Fatal(err)
	}
	actor := IntentActorDTO{UserID: 17}
	initial, err := control.PutDraft(context.Background(), PutCurrentIntentDraftCommand{
		Actor: actor, MonitorID: 7, ExpectedResourceVersion: 0,
		Objective: "Track launch disruption",
		Clauses:   []IntentClauseDTO{{Operator: "must", Field: "action", Value: "launch"}},
	})
	if err != nil {
		t.Fatalf("PutDraft(initialize): %v", err)
	}
	if !initial.Created || initial.Draft.DraftID != 901 || initial.Draft.ResourceVersion != 1 || repository.initializations != 1 {
		t.Fatalf("initial result/repository = %#v / %#v", initial, repository)
	}

	read, err := control.ReadDraft(context.Background(), ReadCurrentIntentDraftQuery{Actor: actor, MonitorID: 7})
	if err != nil || read.Draft.DraftID != initial.Draft.DraftID || read.Draft.Objective != initial.Draft.Objective {
		t.Fatalf("ReadDraft() = %#v / %v", read, err)
	}
	updated, err := control.PutDraft(context.Background(), PutCurrentIntentDraftCommand{
		Actor: actor, MonitorID: 7, ExpectedResourceVersion: 1,
		Objective: "Track launch disruption and recovery",
		Clauses:   []IntentClauseDTO{{Operator: "must", Field: "action", Value: "launch"}},
	})
	if err != nil {
		t.Fatalf("PutDraft(update): %v", err)
	}
	if updated.Created || updated.Draft.DraftID != initial.Draft.DraftID || updated.Draft.ResourceVersion != 2 || repository.initializations != 1 || repository.saves != 1 {
		t.Fatalf("updated result/repository = %#v / %#v", updated, repository)
	}
	if _, err := control.PutDraft(context.Background(), PutCurrentIntentDraftCommand{
		Actor: actor, MonitorID: 7, ExpectedResourceVersion: 1, Objective: "stale update",
	}); !errors.Is(err, ErrIntentVersionConflict) {
		t.Fatalf("stale update error = %v", err)
	}
}

func TestIntentControlAuthorizationAndCandidateDecisionFailClosed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 17, 5, 0, 0, time.UTC)
	draft := intentDraftFixture()
	repository := &intentControlRepositoryFake{intentDraftRepositoryFake: intentDraftRepositoryFake{draft: draft}}
	authorizer := &intentControlAuthorizerFake{roles: map[int64]string{8: "editor", 9: "admin"}}
	intents, _ := NewIntentService(IntentServiceDependencies{Drafts: repository, Runs: newIntentRunRepositoryFake(now), Clock: fixedIntentClock{now: now}})
	control, _ := NewIntentControlService(IntentControlDependencies{Intents: intents, CurrentDrafts: repository, RunStatuses: repository, Authorizer: authorizer})

	if _, err := control.ReadDraft(context.Background(), ReadCurrentIntentDraftQuery{
		Actor: IntentActorDTO{UserID: 10}, MonitorID: draft.MonitorID,
	}); appErrorCode(err) != sharederrors.CodeForbidden {
		t.Fatalf("viewer read error = %v", err)
	}
	if _, err := control.ReviewExpansionCandidate(context.Background(), ReviewCurrentExpansionCandidateCommand{
		Actor: IntentActorDTO{UserID: 8}, MonitorID: draft.MonitorID,
		CandidateID: draft.Candidates[0].ID, ExpectedResourceVersion: draft.ResourceVersion,
		Decision: "approved", IdempotencyKey: "review.editor.denied",
	}); appErrorCode(err) != sharederrors.CodeForbidden {
		t.Fatalf("editor review error = %v", err)
	}

	result, err := control.ReviewExpansionCandidate(context.Background(), ReviewCurrentExpansionCandidateCommand{
		Actor: IntentActorDTO{UserID: 9}, MonitorID: draft.MonitorID,
		CandidateID: draft.Candidates[0].ID, ExpectedResourceVersion: draft.ResourceVersion,
		Decision: "approved", Note: "reviewed", IdempotencyKey: "review.admin.approved",
	})
	if err != nil {
		t.Fatalf("ReviewExpansionCandidate(admin): %v", err)
	}
	if result.Draft.ResourceVersion != draft.ResourceVersion+1 || result.Draft.Candidates[0].ReviewerUserID == nil || *result.Draft.Candidates[0].ReviewerUserID != 9 {
		t.Fatalf("review result = %#v", result)
	}
}

func TestIntentControlReadsHistoricalRunByMonitorAndExactStoredDraftIdentity(t *testing.T) {
	t.Parallel()

	queuedAt := time.Date(2026, 8, 9, 17, 10, 0, 0, time.UTC)
	startedAt, completedAt, invalidatedAt := queuedAt.Add(time.Second), queuedAt.Add(2*time.Second), queuedAt.Add(3*time.Second)
	run := ExpansionRunDTO{Run: IntentRunDTO{
		ID: 501, Kind: "expansion", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
		InputHash: strings.Repeat("a", 64), Status: "invalidated", QueuedAt: queuedAt,
		StartedAt: &startedAt, CompletedAt: &completedAt, InvalidatedAt: &invalidatedAt,
	}}
	repository := &intentControlRepositoryFake{
		intentDraftRepositoryFake: intentDraftRepositoryFake{draft: intentDraftFixture()},
		expansionStatus:           run,
	}
	authorizer := &intentControlAuthorizerFake{roles: map[int64]string{2: "editor"}}
	runs := newIntentRunRepositoryFake(queuedAt)
	runs.expansion = run
	intents, _ := NewIntentService(IntentServiceDependencies{Drafts: repository, Runs: runs, Clock: fixedIntentClock{now: queuedAt}})
	control, _ := NewIntentControlService(IntentControlDependencies{Intents: intents, CurrentDrafts: repository, RunStatuses: repository, Authorizer: authorizer})

	result, err := control.ReadExpansionRun(context.Background(), ReadIntentExpansionRunQuery{
		Actor: IntentActorDTO{UserID: 2}, MonitorID: 7, RunID: 501,
	})
	if err != nil {
		t.Fatalf("ReadExpansionRun(): %v", err)
	}
	if result.Expansion.Run.DraftID != 101 || result.Expansion.Run.DraftResourceVersion != 4 || repository.lastStatusLookup.RunID != 501 {
		t.Fatalf("historical run/lookup = %#v / %#v", result, repository.lastStatusLookup)
	}

	repository.expansionStatus.Run.MonitorID = 8
	if _, err := control.ReadExpansionRun(context.Background(), ReadIntentExpansionRunQuery{
		Actor: IntentActorDTO{UserID: 2}, MonitorID: 7, RunID: 501,
	}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("cross-monitor repository result error = %v", err)
	}
}

func TestIntentControlSubmitChecksDurableAuthorizationThenCapabilityBeforeReservation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 17, 20, 0, 0, time.UTC)
	repository := &intentControlRepositoryFake{intentDraftRepositoryFake: intentDraftRepositoryFake{draft: intentDraftFixture()}}
	runs := newIntentRunRepositoryFake(now)
	intents, _ := NewIntentService(IntentServiceDependencies{Drafts: repository, Runs: runs, Clock: fixedIntentClock{now: now}})
	authorizer := &intentControlAuthorizerFake{roles: map[int64]string{9: "admin"}}
	control, _ := NewIntentControlService(IntentControlDependencies{
		Intents: intents, CurrentDrafts: repository, RunStatuses: repository, Authorizer: authorizer,
	})
	command := SubmitCurrentExpansionRunCommand{
		Actor: IntentActorDTO{UserID: 9}, MonitorID: 7, ExpectedResourceVersion: 4,
		IdempotencyKey: "expand-unavailable", ExpansionProfile: "monitor-intent-expansion-v1",
	}
	if _, err := control.SubmitExpansionRun(context.Background(), command); appErrorCode(err) != sharederrors.CodeAIModelUnavailable || runs.reservations != 0 {
		t.Fatalf("unavailable submit error/reservations = %v/%d", err, runs.reservations)
	}
	delete(authorizer.roles, 9)
	if _, err := control.SubmitExpansionRun(context.Background(), command); appErrorCode(err) != sharederrors.CodeForbidden || runs.reservations != 0 {
		t.Fatalf("revoked replay error/reservations = %v/%d", err, runs.reservations)
	}
}

type intentControlAuthorizerFake struct {
	roles map[int64]string
}

func (authorizer *intentControlAuthorizerFake) AuthorizeIntentControl(_ context.Context, query AuthorizeIntentControlQueryDTO) error {
	role := authorizer.roles[query.ActorUserID]
	if role == "admin" {
		return nil
	}
	switch query.Operation {
	case IntentControlReadDraft, IntentControlReplaceDraft, IntentControlReadExpansion, IntentControlSubmitPreview, IntentControlReadPreview:
		if role == "editor" {
			return nil
		}
	}
	return ErrIntentAuthorizationDenied
}

type intentControlRepositoryFake struct {
	intentDraftRepositoryFake
	initializations  int
	expansionStatus  ExpansionRunDTO
	previewStatus    PreviewRunDTO
	lastStatusLookup IntentRunStatusLookupDTO
}

func (repository *intentControlRepositoryFake) FindCurrent(_ context.Context, query ReadCurrentIntentDraftRepositoryQuery) (IntentDraftDTO, error) {
	if repository.draft.MonitorID == 0 || repository.draft.MonitorID != query.MonitorID {
		return IntentDraftDTO{}, ErrIntentDraftNotFound
	}
	return cloneIntentDraftDTO(repository.draft), nil
}

func (repository *intentControlRepositoryFake) InitializeCurrent(_ context.Context, mutation InitializeCurrentIntentDraftMutationDTO) (IntentDraftDTO, error) {
	if repository.draft.MonitorID != 0 {
		return IntentDraftDTO{}, ErrIntentVersionConflict
	}
	repository.initializations++
	repository.draft = cloneIntentDraftDTO(mutation.Initial)
	repository.draft.DraftID = 901
	return cloneIntentDraftDTO(repository.draft), nil
}

func (repository *intentControlRepositoryFake) FindExpansionStatus(_ context.Context, lookup IntentRunStatusLookupDTO) (ExpansionRunDTO, error) {
	repository.lastStatusLookup = lookup
	if repository.expansionStatus.Run.ID == 0 {
		return ExpansionRunDTO{}, ErrIntentRunNotFound
	}
	return cloneExpansionRunDTO(repository.expansionStatus), nil
}

func (repository *intentControlRepositoryFake) FindPreviewStatus(_ context.Context, lookup IntentRunStatusLookupDTO) (PreviewRunDTO, error) {
	repository.lastStatusLookup = lookup
	if repository.previewStatus.Run.ID == 0 {
		return PreviewRunDTO{}, ErrIntentRunNotFound
	}
	return clonePreviewRunDTO(repository.previewStatus), nil
}

func appErrorCode(err error) int {
	var app *sharederrors.AppError
	if errors.As(err, &app) {
		return app.Code
	}
	return 0
}
