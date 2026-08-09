package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIntentServiceAtomicallyReplacesDraftAndInvalidatesRuns(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 13, 0, 0, 0, time.UTC)
	drafts := &intentDraftRepositoryFake{draft: intentDraftFixture()}
	runs := newIntentRunRepositoryFake(now)
	service, err := NewIntentService(IntentServiceDependencies{Drafts: drafts, Runs: runs, Clock: fixedIntentClock{now: now}})
	if err != nil {
		t.Fatalf("NewIntentService(): %v", err)
	}

	result, err := service.ReplaceDraft(context.Background(), ReplaceIntentDraftCommand{
		MonitorID: 7, DraftID: 101, ExpectedResourceVersion: 4,
		Objective: "  Track robotics acquisitions  ",
		Clauses: []IntentClauseDTO{
			{Operator: "must", Field: "term", Value: "robotics company"},
			{Operator: "must_not", Field: "term", Value: "jobs"},
		},
		Entities: []IntentEntityDTO{{CanonicalID: "wikidata:Q2283", DisplayName: "Microsoft", Aliases: []string{"微软"}}},
		Examples: []IntentExampleDTO{{Label: "positive", Text: "Microsoft buys a robotics startup"}},
	})
	if err != nil {
		t.Fatalf("ReplaceDraft(): %v", err)
	}
	if result.Draft.ResourceVersion != 5 || result.Draft.Objective != "Track robotics acquisitions" || len(result.Draft.Candidates) != 0 {
		t.Fatalf("replacement result = %#v", result.Draft)
	}
	if drafts.saves != 1 || drafts.lastMutation.Kind != IntentDraftMutationReplace || !drafts.lastMutation.InvalidatedAt.Equal(now) {
		t.Fatalf("mutation = saves %d %#v", drafts.saves, drafts.lastMutation)
	}
	if drafts.lastMutation.ExpectedResourceVersion != 4 {
		t.Fatalf("mutation expected version = %d", drafts.lastMutation.ExpectedResourceVersion)
	}
	if drafts.lastMutation.ExpectedDraftID != 101 {
		t.Fatalf("mutation expected draft id = %d", drafts.lastMutation.ExpectedDraftID)
	}

	_, err = service.ReplaceDraft(context.Background(), ReplaceIntentDraftCommand{
		MonitorID: 7, DraftID: 101, ExpectedResourceVersion: 4, Objective: "stale",
	})
	if !errors.Is(err, ErrIntentVersionConflict) || drafts.saves != 1 {
		t.Fatalf("stale replacement = %v, saves %d", err, drafts.saves)
	}
}

func TestIntentServiceReviewsCandidateWithoutExposingDomainEntity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	drafts := &intentDraftRepositoryFake{draft: intentDraftFixture()}
	service, err := NewIntentService(IntentServiceDependencies{Drafts: drafts, Runs: newIntentRunRepositoryFake(now), Clock: fixedIntentClock{now: now}})
	if err != nil {
		t.Fatalf("NewIntentService(): %v", err)
	}
	command := ReviewExpansionCandidateCommand{
		MonitorID: 7, DraftID: 101, CandidateID: "candidate-1", ExpectedResourceVersion: 4,
		Decision: "approved", ReviewerUserID: 88, Note: "matches positive examples", IdempotencyKey: "candidate-review-once",
	}
	result, err := service.ReviewCandidate(context.Background(), command)
	if err != nil {
		t.Fatalf("ReviewCandidate(): %v", err)
	}
	if result.Reused || result.Draft.ResourceVersion != 5 || len(result.Draft.Candidates) != 1 {
		t.Fatalf("review result = %#v", result.Draft)
	}
	candidate := result.Draft.Candidates[0]
	if candidate.ApprovalStatus != "approved" || candidate.ReviewerUserID == nil || *candidate.ReviewerUserID != 88 || candidate.ReviewedAt == nil || !candidate.ReviewedAt.Equal(now) {
		t.Fatalf("candidate review DTO = %#v", candidate)
	}
	if drafts.lastMutation.Kind != IntentDraftMutationCandidateReview {
		t.Fatalf("mutation kind = %q", drafts.lastMutation.Kind)
	}
	retried, err := service.ReviewCandidate(context.Background(), command)
	if err != nil || !retried.Reused || retried.Draft.ResourceVersion != 5 || drafts.saves != 1 {
		t.Fatalf("idempotent review retry = %#v/%v saves=%d", retried, err, drafts.saves)
	}
	conflicting := command
	conflicting.Decision = "rejected"
	if _, err := service.ReviewCandidate(context.Background(), conflicting); !errors.Is(err, ErrIntentIdempotencyConflict) {
		t.Fatalf("same review key with different input = %v", err)
	}

	// DTO mutation cannot reach the repository/domain snapshot through a
	// mapper-owned collection.
	result.Draft.Candidates[0].Value = "tampered"
	read, err := service.ReadDraft(context.Background(), ReadIntentDraftQuery{MonitorID: 7, DraftID: 101})
	if err != nil {
		t.Fatalf("ReadDraft(): %v", err)
	}
	if read.Draft.Candidates[0].Value == "tampered" {
		t.Fatal("application DTO mutated persisted draft state")
	}
}

func TestIntentServiceSubmitsVersionBoundIdempotentExpansionAndPreviewRuns(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)
	drafts := &intentDraftRepositoryFake{draft: intentDraftFixture()}
	runs := newIntentRunRepositoryFake(now)
	service, err := NewIntentService(IntentServiceDependencies{Drafts: drafts, Runs: runs, Clock: fixedIntentClock{now: now}})
	if err != nil {
		t.Fatalf("NewIntentService(): %v", err)
	}

	expansionCommand := SubmitExpansionRunCommand{
		MonitorID: 7, DraftID: 101, ExpectedResourceVersion: 4,
		IdempotencyKey: "expand-once", ExpansionProfile: "monitor-intent-expansion-v1",
	}
	first, err := service.SubmitExpansionRun(context.Background(), expansionCommand)
	if err != nil {
		t.Fatalf("SubmitExpansionRun(first): %v", err)
	}
	second, err := service.SubmitExpansionRun(context.Background(), expansionCommand)
	if err != nil {
		t.Fatalf("SubmitExpansionRun(second): %v", err)
	}
	if first.Reused || !second.Reused || first.Run.ID != second.Run.ID || first.Run.InputHash == "" {
		t.Fatalf("idempotent results = %#v %#v", first, second)
	}
	if first.Run.Kind != "expansion" || first.Run.DraftID != 101 || first.Run.DraftResourceVersion != 4 || runs.reservations != 2 || runs.created != 1 {
		t.Fatalf("expansion run/reservations = %#v %d/%d", first.Run, runs.reservations, runs.created)
	}

	_, err = service.SubmitExpansionRun(context.Background(), SubmitExpansionRunCommand{
		MonitorID: 7, DraftID: 101, ExpectedResourceVersion: 4,
		IdempotencyKey: "expand-once", ExpansionProfile: "different-profile",
	})
	if !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("unsupported expansion parameter profile = %v", err)
	}

	preview, err := service.SubmitPreviewRun(context.Background(), SubmitPreviewRunCommand{
		MonitorID: 7, DraftID: 101, ExpectedResourceVersion: 4,
		IdempotencyKey: "preview-once", EvaluatorProfile: "intent-preview-v1", SampleLimit: 50,
	})
	if err != nil {
		t.Fatalf("SubmitPreviewRun(): %v", err)
	}
	if preview.Run.Kind != "preview" || preview.Run.InputHash == first.Run.InputHash {
		t.Fatalf("preview run = %#v, expansion hash %s", preview.Run, first.Run.InputHash)
	}
	newDraftDTO := intentDraftFixture()
	newDraftDTO.DraftID = 102
	conflictingDraftService, _ := NewIntentService(IntentServiceDependencies{Drafts: &intentDraftRepositoryFake{draft: newDraftDTO}, Runs: runs, Clock: fixedIntentClock{now: now}})
	if _, err := conflictingDraftService.SubmitExpansionRun(context.Background(), SubmitExpansionRunCommand{
		MonitorID: 7, DraftID: 102, ExpectedResourceVersion: 4,
		IdempotencyKey: "expand-once", ExpansionProfile: "monitor-intent-expansion-v1",
	}); !errors.Is(err, ErrIntentIdempotencyConflict) {
		t.Fatalf("same run key reused for another draft identity = %v", err)
	}
	newDraftRuns := newIntentRunRepositoryFake(now)
	newDraftService, _ := NewIntentService(IntentServiceDependencies{Drafts: &intentDraftRepositoryFake{draft: newDraftDTO}, Runs: newDraftRuns, Clock: fixedIntentClock{now: now}})
	newDraftRun, err := newDraftService.SubmitExpansionRun(context.Background(), SubmitExpansionRunCommand{
		MonitorID: 7, DraftID: 102, ExpectedResourceVersion: 4,
		IdempotencyKey: "new-draft", ExpansionProfile: "monitor-intent-expansion-v1",
	})
	if err != nil {
		t.Fatalf("SubmitExpansionRun(new draft identity): %v", err)
	}
	if newDraftRun.Run.InputHash == first.Run.InputHash || newDraftRun.Run.DraftID != 102 {
		t.Fatalf("new draft reused old run identity: old=%#v new=%#v", first.Run, newDraftRun.Run)
	}

	before := runs.reservations
	_, err = service.SubmitPreviewRun(context.Background(), SubmitPreviewRunCommand{
		MonitorID: 7, DraftID: 101, ExpectedResourceVersion: 3,
		IdempotencyKey: "stale-preview", EvaluatorProfile: "intent-preview-v1", SampleLimit: 50,
	})
	if !errors.Is(err, ErrIntentVersionConflict) || runs.reservations != before {
		t.Fatalf("stale preview = %v reservations %d/%d", err, before, runs.reservations)
	}
}

func TestIntentServiceStartsAndFailsRunsThroughCAS(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 15, 30, 0, 0, time.UTC)
	drafts := &intentDraftRepositoryFake{draft: intentDraftFixture()}
	runs := newIntentRunRepositoryFake(now)
	runs.expansion = ExpansionRunDTO{Run: IntentRunDTO{
		ID: 301, Kind: "expansion", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
		InputHash: strings.Repeat("b", 64), Status: "queued", QueuedAt: now.Add(-time.Minute),
	}}
	service, _ := NewIntentService(IntentServiceDependencies{Drafts: drafts, Runs: runs, Clock: fixedIntentClock{now: now}})
	reference := intentRunReferenceFixture(runs.expansion.Run)

	started, err := service.StartIntentRun(context.Background(), StartIntentRunCommand{Run: reference})
	if err != nil {
		t.Fatalf("StartIntentRun(): %v", err)
	}
	if started.Reused || started.Run.Status != "running" || started.Run.StartedAt == nil || !started.Run.StartedAt.Equal(now) {
		t.Fatalf("started run = %#v", started)
	}
	if runs.transitions != 1 || runs.lastTransition.Expected.Status != "queued" || runs.lastTransition.Next.Status != "running" {
		t.Fatalf("start transition = %#v", runs.lastTransition)
	}

	retried, err := service.StartIntentRun(context.Background(), StartIntentRunCommand{Run: reference})
	if err != nil || !retried.Reused || runs.transitions != 1 {
		t.Fatalf("start retry = %#v/%v transitions=%d", retried, err, runs.transitions)
	}

	failedAt := now.Add(time.Second)
	service.clock = fixedIntentClock{now: failedAt}
	failed, err := service.FailIntentRun(context.Background(), FailIntentRunCommand{Run: reference, Reason: "provider_timeout"})
	if err != nil {
		t.Fatalf("FailIntentRun(): %v", err)
	}
	if failed.Reused || failed.Run.Status != "failed" || failed.Run.FailureReason != "provider_timeout" || failed.Run.CompletedAt == nil || !failed.Run.CompletedAt.Equal(failedAt) {
		t.Fatalf("failed run = %#v", failed)
	}
	if runs.transitions != 2 || runs.lastTransition.Expected.Status != "running" || runs.lastTransition.Next.Status != "failed" {
		t.Fatalf("fail transition = %#v", runs.lastTransition)
	}
	retriedFailure, err := service.FailIntentRun(context.Background(), FailIntentRunCommand{Run: reference, Reason: "provider_timeout"})
	if err != nil || !retriedFailure.Reused || runs.transitions != 2 {
		t.Fatalf("failure retry = %#v/%v transitions=%d", retriedFailure, err, runs.transitions)
	}
	if _, err := service.FailIntentRun(context.Background(), FailIntentRunCommand{Run: reference, Reason: "different_failure"}); !errors.Is(err, ErrIntentRunResultConflict) {
		t.Fatalf("different terminal failure reason = %v", err)
	}
}

func TestIntentServiceAtomicallyCompletesExpansionAndAdvancesDraft(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 15, 45, 0, 0, time.UTC)
	startedAt := now.Add(-time.Second)
	drafts := &intentDraftRepositoryFake{draft: intentDraftFixture()}
	runs := newIntentRunRepositoryFake(now)
	runs.drafts = drafts
	runs.expansion = ExpansionRunDTO{Run: IntentRunDTO{
		ID: 302, Kind: "expansion", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
		InputHash: strings.Repeat("b", 64), Status: "running", QueuedAt: now.Add(-time.Minute), StartedAt: &startedAt,
	}}
	service, _ := NewIntentService(IntentServiceDependencies{Drafts: drafts, Runs: runs, Clock: fixedIntentClock{now: now}})
	candidate := ExpansionCandidateDTO{
		ID: "candidate-result", Value: "merger", Source: "llm", Reason: "semantic neighbor",
		ModelVersion: "model-v1", PromptVersion: "prompt-v1", InputHash: strings.Repeat("b", 64),
		Similarity: 0.8, Risk: "low", ApprovalStatus: "pending",
	}

	result, err := service.CompleteExpansionRun(context.Background(), CompleteExpansionRunCommand{
		Run: intentRunReferenceFixture(runs.expansion.Run), Candidates: []ExpansionCandidateDTO{candidate},
	})
	if err != nil {
		t.Fatalf("CompleteExpansionRun(): %v", err)
	}
	if result.Reused || result.Expansion.Run.Status != "invalidated" || result.Expansion.Run.CompletedAt == nil || result.Expansion.Run.InvalidatedAt == nil {
		t.Fatalf("completed expansion = %#v", result)
	}
	if runs.expansionCompletions != 1 || runs.lastExpansionCompletion.Transition.Expected.Status != "running" || runs.lastExpansionCompletion.Transition.Next.Status != "invalidated" {
		t.Fatalf("expansion completion = %#v", runs.lastExpansionCompletion)
	}
	if runs.lastExpansionCompletion.DraftMutation.Kind != IntentDraftMutationExpansionResult || runs.lastExpansionCompletion.DraftMutation.ExpectedResourceVersion != 4 {
		t.Fatalf("draft completion mutation = %#v", runs.lastExpansionCompletion.DraftMutation)
	}
	if drafts.draft.ResourceVersion != 5 || len(drafts.draft.Candidates) != 2 || drafts.draft.Candidates[1].ID != "candidate-result" {
		t.Fatalf("advanced draft = %#v", drafts.draft)
	}

	retried, err := service.CompleteExpansionRun(context.Background(), CompleteExpansionRunCommand{
		Run: intentRunReferenceFixture(runs.expansion.Run), Candidates: []ExpansionCandidateDTO{candidate},
	})
	if err != nil || !retried.Reused || runs.expansionCompletions != 1 {
		t.Fatalf("expansion completion retry = %#v/%v completions=%d", retried, err, runs.expansionCompletions)
	}
	conflicting := candidate
	conflicting.Value = "different"
	if _, err := service.CompleteExpansionRun(context.Background(), CompleteExpansionRunCommand{
		Run: intentRunReferenceFixture(runs.expansion.Run), Candidates: []ExpansionCandidateDTO{conflicting},
	}); !errors.Is(err, ErrIntentRunResultConflict) {
		t.Fatalf("different expansion retry output = %v", err)
	}
}

func TestIntentServiceCompletesPreviewWithRequiredResult(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 15, 50, 0, 0, time.UTC)
	startedAt := now.Add(-time.Second)
	runs := newIntentRunRepositoryFake(now)
	runs.preview = PreviewRunDTO{Run: IntentRunDTO{
		ID: 303, Kind: "preview", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
		InputHash: strings.Repeat("c", 64), Status: "running", QueuedAt: now.Add(-time.Minute), StartedAt: &startedAt,
	}}
	service, _ := NewIntentService(IntentServiceDependencies{Drafts: &intentDraftRepositoryFake{draft: intentDraftFixture()}, Runs: runs, Clock: fixedIntentClock{now: now}})
	preview := IntentPreviewDTO{EstimatedAlertCount: 1, Samples: []PreviewSampleDTO{{
		DocumentVersionID: 91, Title: "Candidate", Decision: "review",
		RecallSignals: []PreviewRecallSignalDTO{{Channel: "lexical", Rank: 1, Score: 3.1}},
	}}}

	result, err := service.CompletePreviewRun(context.Background(), CompletePreviewRunCommand{
		Run: intentRunReferenceFixture(runs.preview.Run), Preview: preview,
	})
	if err != nil {
		t.Fatalf("CompletePreviewRun(): %v", err)
	}
	if result.Reused || result.Preview.Run.Status != "succeeded" || result.Preview.Preview == nil {
		t.Fatalf("completed preview = %#v", result)
	}
	if runs.previewCompletions != 1 || runs.lastPreviewCompletion.Transition.Next.Status != "succeeded" {
		t.Fatalf("preview completion = %#v", runs.lastPreviewCompletion)
	}

	retried, err := service.CompletePreviewRun(context.Background(), CompletePreviewRunCommand{
		Run: intentRunReferenceFixture(runs.preview.Run), Preview: preview,
	})
	if err != nil || !retried.Reused || runs.previewCompletions != 1 {
		t.Fatalf("preview completion retry = %#v/%v completions=%d", retried, err, runs.previewCompletions)
	}
	if _, err := service.CompletePreviewRun(context.Background(), CompletePreviewRunCommand{
		Run: intentRunReferenceFixture(runs.preview.Run), Preview: IntentPreviewDTO{EstimatedAlertCount: 2},
	}); !errors.Is(err, ErrIntentRunResultConflict) {
		t.Fatalf("different preview retry output = %v", err)
	}
}

func TestIntentServiceRejectsCorruptRepositoryDTO(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	draft := intentDraftFixture()
	draft.Candidates[0].InputHash = "corrupt"
	drafts := &intentDraftRepositoryFake{draft: draft}
	service, err := NewIntentService(IntentServiceDependencies{Drafts: drafts, Runs: newIntentRunRepositoryFake(now), Clock: fixedIntentClock{now: now}})
	if err != nil {
		t.Fatalf("NewIntentService(): %v", err)
	}
	if _, err := service.ReadDraft(context.Background(), ReadIntentDraftQuery{MonitorID: 7, DraftID: 101}); err == nil {
		t.Fatal("corrupt persistence DTO was returned as a valid draft")
	}
}

func TestIntentServiceReadsValidatedExpansionAndPreviewResults(t *testing.T) {
	t.Parallel()

	queuedAt := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	startedAt := queuedAt.Add(time.Second)
	completedAt := queuedAt.Add(2 * time.Second)
	runs := newIntentRunRepositoryFake(queuedAt)
	runs.expansion = ExpansionRunDTO{
		Run: IntentRunDTO{
			ID: 201, Kind: "expansion", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
			InputHash: strings.Repeat("b", 64), Status: "succeeded", QueuedAt: queuedAt,
			StartedAt: &startedAt, CompletedAt: &completedAt,
		},
		Candidates: []ExpansionCandidateDTO{{
			ID: "candidate-result", Value: "merger", Source: "llm", Reason: "semantic neighbor",
			ModelVersion: "model-v1", PromptVersion: "prompt-v1", InputHash: strings.Repeat("b", 64),
			Similarity: 0.8, Risk: "low", ApprovalStatus: "pending",
		}},
	}
	runs.preview = PreviewRunDTO{
		Run: IntentRunDTO{
			ID: 202, Kind: "preview", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
			InputHash: strings.Repeat("c", 64), Status: "succeeded", QueuedAt: queuedAt,
			StartedAt: &startedAt, CompletedAt: &completedAt,
		},
		Preview: &IntentPreviewDTO{
			EstimatedAlertCount: 3, Warnings: []string{"uncalibrated"},
			Samples: []PreviewSampleDTO{{
				DocumentVersionID: 101, Title: "Example", Decision: "review",
				RecallSignals: []PreviewRecallSignalDTO{{Channel: "lexical", Rank: 1, Score: 0.8}},
				Reasons:       []string{"must_match"}, ExclusionReasons: []string{},
			}},
		},
	}
	service, err := NewIntentService(IntentServiceDependencies{Drafts: &intentDraftRepositoryFake{draft: intentDraftFixture()}, Runs: runs, Clock: fixedIntentClock{now: queuedAt}})
	if err != nil {
		t.Fatalf("NewIntentService(): %v", err)
	}
	expansion, err := service.ReadExpansionRun(context.Background(), ReadExpansionRunQuery{MonitorID: 7, DraftID: 101, DraftResourceVersion: 4, RunID: 201})
	if err != nil {
		t.Fatalf("ReadExpansionRun(): %v", err)
	}
	if len(expansion.Expansion.Candidates) != 1 || expansion.Expansion.Candidates[0].ApprovalStatus != "pending" {
		t.Fatalf("expansion result = %#v", expansion)
	}
	if _, err := service.ReadExpansionRun(context.Background(), ReadExpansionRunQuery{MonitorID: 7, DraftID: 102, DraftResourceVersion: 4, RunID: 201}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("run accepted under another draft identity: %v", err)
	}
	runs.expansion.Candidates[0].InputHash = strings.Repeat("e", 64)
	if _, err := service.ReadExpansionRun(context.Background(), ReadExpansionRunQuery{MonitorID: 7, DraftID: 101, DraftResourceVersion: 4, RunID: 201}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("candidate from another run input was accepted: %v", err)
	}
	preview, err := service.ReadPreviewRun(context.Background(), ReadPreviewRunQuery{MonitorID: 7, DraftID: 101, DraftResourceVersion: 4, RunID: 202})
	if err != nil {
		t.Fatalf("ReadPreviewRun(): %v", err)
	}
	if preview.Preview.Preview == nil || preview.Preview.Preview.Samples[0].RecallSignals[0].Channel != "lexical" {
		t.Fatalf("preview result = %#v", preview)
	}

	preview.Preview.Preview.Samples[0].Reasons[0] = "tampered"
	again, err := service.ReadPreviewRun(context.Background(), ReadPreviewRunQuery{MonitorID: 7, DraftID: 101, DraftResourceVersion: 4, RunID: 202})
	if err != nil || again.Preview.Preview.Samples[0].Reasons[0] == "tampered" {
		t.Fatalf("preview DTO aliasing = %#v/%v", again, err)
	}
}

func TestIntentServiceRejectsResultPayloadBeforeRunSuccess(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 17, 0, 0, 0, time.UTC)
	runs := newIntentRunRepositoryFake(now)
	runs.expansion = ExpansionRunDTO{
		Run: IntentRunDTO{
			ID: 203, Kind: "expansion", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
			InputHash: strings.Repeat("d", 64), Status: "queued", QueuedAt: now,
		},
		Candidates: []ExpansionCandidateDTO{intentDraftFixture().Candidates[0]},
	}
	service, _ := NewIntentService(IntentServiceDependencies{Drafts: &intentDraftRepositoryFake{draft: intentDraftFixture()}, Runs: runs, Clock: fixedIntentClock{now: now}})
	if _, err := service.ReadExpansionRun(context.Background(), ReadExpansionRunQuery{MonitorID: 7, DraftID: 101, DraftResourceVersion: 4, RunID: 203}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("unfinished result payload = %v", err)
	}
	runs.preview = PreviewRunDTO{Run: IntentRunDTO{
		ID: 204, Kind: "preview", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
		InputHash: strings.Repeat("e", 64), Status: "succeeded", QueuedAt: now,
		StartedAt: timePointer(now.Add(time.Second)), CompletedAt: timePointer(now.Add(2 * time.Second)),
	}}
	if _, err := service.ReadPreviewRun(context.Background(), ReadPreviewRunQuery{MonitorID: 7, DraftID: 101, DraftResourceVersion: 4, RunID: 204}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("successful preview without a result = %v", err)
	}
}

func TestIntentServiceKeepsSuccessfulResultsReadableAfterInvalidation(t *testing.T) {
	t.Parallel()

	queuedAt := time.Date(2026, 8, 9, 17, 30, 0, 0, time.UTC)
	startedAt := queuedAt.Add(time.Second)
	completedAt := queuedAt.Add(2 * time.Second)
	invalidatedAt := queuedAt.Add(3 * time.Second)
	runs := newIntentRunRepositoryFake(queuedAt)
	runs.expansion = ExpansionRunDTO{
		Run: IntentRunDTO{
			ID: 205, Kind: "expansion", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
			InputHash: strings.Repeat("f", 64), Status: "invalidated", QueuedAt: queuedAt,
			StartedAt: &startedAt, CompletedAt: &completedAt, InvalidatedAt: &invalidatedAt,
		},
		Candidates: []ExpansionCandidateDTO{{
			ID: "historical", Value: "takeover", Source: "llm", Reason: "semantic neighbor",
			ModelVersion: "model-v1", PromptVersion: "prompt-v1", InputHash: strings.Repeat("f", 64),
			Similarity: 0.8, Risk: "low", ApprovalStatus: "pending",
		}},
	}
	runs.preview = PreviewRunDTO{
		Run: IntentRunDTO{
			ID: 206, Kind: "preview", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
			InputHash: strings.Repeat("e", 64), Status: "invalidated", QueuedAt: queuedAt,
			StartedAt: &startedAt, CompletedAt: &completedAt, InvalidatedAt: &invalidatedAt,
		},
		Preview: &IntentPreviewDTO{EstimatedAlertCount: 2},
	}
	service, _ := NewIntentService(IntentServiceDependencies{Drafts: &intentDraftRepositoryFake{draft: intentDraftFixture()}, Runs: runs, Clock: fixedIntentClock{now: queuedAt}})

	expansion, err := service.ReadExpansionRun(context.Background(), ReadExpansionRunQuery{MonitorID: 7, DraftID: 101, DraftResourceVersion: 4, RunID: 205})
	if err != nil || len(expansion.Expansion.Candidates) != 1 {
		t.Fatalf("invalidated successful expansion = %#v/%v", expansion, err)
	}
	preview, err := service.ReadPreviewRun(context.Background(), ReadPreviewRunQuery{MonitorID: 7, DraftID: 101, DraftResourceVersion: 4, RunID: 206})
	if err != nil || preview.Preview.Preview == nil {
		t.Fatalf("invalidated successful preview = %#v/%v", preview, err)
	}

	// A run invalidated before completing has no result visibility.
	runs.preview.Run.StartedAt = nil
	runs.preview.Run.CompletedAt = nil
	if _, err := service.ReadPreviewRun(context.Background(), ReadPreviewRunQuery{MonitorID: 7, DraftID: 101, DraftResourceVersion: 4, RunID: 206}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("unfinished invalidated preview exposed payload: %v", err)
	}
}

func intentDraftFixture() IntentDraftDTO {
	reviewer := int64(0)
	_ = reviewer
	return IntentDraftDTO{
		MonitorID: 7, DraftID: 101, ResourceVersion: 4, Objective: "Track AI acquisitions",
		Clauses: []IntentClauseDTO{
			{Operator: "must", Field: "action", Value: "acquisition"},
			{Operator: "must_not", Field: "term", Value: "jobs"},
		},
		Entities: []IntentEntityDTO{}, Examples: []IntentExampleDTO{
			{Label: "positive", Text: "An AI company is acquired"},
			{Label: "negative", Text: "An AI company posts jobs"},
		},
		Candidates: []ExpansionCandidateDTO{{
			ID: "candidate-1", Value: "takeover", Source: "llm", Reason: "semantic neighbor",
			ModelVersion: "model-v1", PromptVersion: "prompt-v1", InputHash: strings.Repeat("a", 64),
			Similarity: 0.82, Risk: "medium", ApprovalStatus: "pending",
		}},
	}
}

func intentRunReferenceFixture(run IntentRunDTO) IntentRunReferenceDTO {
	return IntentRunReferenceDTO{
		RunID: run.ID, Kind: run.Kind, MonitorID: run.MonitorID, DraftID: run.DraftID,
		DraftResourceVersion: run.DraftResourceVersion, InputHash: run.InputHash,
	}
}

func timePointer(value time.Time) *time.Time { return &value }

type fixedIntentClock struct{ now time.Time }

func (clock fixedIntentClock) Now() time.Time { return clock.now }

type intentDraftRepositoryFake struct {
	draft        IntentDraftDTO
	saves        int
	lastMutation IntentDraftMutationDTO
	mutations    map[string]IntentDraftMutationReceiptDTO
}

func (repository *intentDraftRepositoryFake) Find(_ context.Context, query ReadIntentDraftQuery) (IntentDraftDTO, error) {
	if repository.draft.MonitorID != query.MonitorID || repository.draft.DraftID != query.DraftID {
		return IntentDraftDTO{}, ErrIntentDraftNotFound
	}
	return cloneIntentDraftDTO(repository.draft), nil
}

func (repository *intentDraftRepositoryFake) FindMutation(_ context.Context, lookup IntentDraftMutationLookupDTO) (IntentDraftMutationReceiptDTO, error) {
	receipt, exists := repository.mutations[lookup.IdempotencyKey]
	if !exists || receipt.Draft.MonitorID != lookup.MonitorID || receipt.Draft.DraftID != lookup.DraftID {
		return IntentDraftMutationReceiptDTO{}, ErrIntentMutationNotFound
	}
	receipt.Created = false
	receipt.Draft = cloneIntentDraftDTO(receipt.Draft)
	return receipt, nil
}

func (repository *intentDraftRepositoryFake) SaveAndInvalidateRuns(_ context.Context, mutation IntentDraftMutationDTO) (IntentDraftMutationReceiptDTO, error) {
	if repository.mutations == nil {
		repository.mutations = make(map[string]IntentDraftMutationReceiptDTO)
	}
	if mutation.IdempotencyKey != "" {
		if prior, exists := repository.mutations[mutation.IdempotencyKey]; exists {
			if prior.CommandFingerprint != mutation.CommandFingerprint {
				return IntentDraftMutationReceiptDTO{}, ErrIntentIdempotencyConflict
			}
			prior.Created = false
			prior.Draft = cloneIntentDraftDTO(prior.Draft)
			return prior, nil
		}
	}
	if mutation.ExpectedDraftID != repository.draft.DraftID || mutation.ExpectedResourceVersion != repository.draft.ResourceVersion {
		return IntentDraftMutationReceiptDTO{}, ErrIntentVersionConflict
	}
	repository.saves++
	repository.lastMutation = mutation
	repository.draft = cloneIntentDraftDTO(mutation.Next)
	receipt := IntentDraftMutationReceiptDTO{
		Draft: cloneIntentDraftDTO(repository.draft), CommandFingerprint: mutation.CommandFingerprint, Created: true,
	}
	if mutation.IdempotencyKey != "" {
		repository.mutations[mutation.IdempotencyKey] = receipt
	}
	return receipt, nil
}

type intentRunRepositoryFake struct {
	now                     time.Time
	drafts                  *intentDraftRepositoryFake
	reservations            int
	created                 int
	transitions             int
	expansionCompletions    int
	previewCompletions      int
	byKey                   map[string]IntentRunReservationDTO
	requestHash             map[string]string
	expansion               ExpansionRunDTO
	preview                 PreviewRunDTO
	lastTransition          IntentRunTransitionDTO
	lastExpansionCompletion CompleteExpansionRunMutationDTO
	lastPreviewCompletion   CompletePreviewRunMutationDTO
}

func newIntentRunRepositoryFake(now time.Time) *intentRunRepositoryFake {
	return &intentRunRepositoryFake{now: now, byKey: map[string]IntentRunReservationDTO{}, requestHash: map[string]string{}}
}

func (repository *intentRunRepositoryFake) ReserveAndEnqueue(_ context.Context, reservation ReserveIntentRunDTO) (IntentRunReservationDTO, error) {
	repository.reservations++
	if priorHash, exists := repository.requestHash[reservation.IdempotencyKey]; exists {
		if priorHash != reservation.RequestHash {
			return IntentRunReservationDTO{}, ErrIntentIdempotencyConflict
		}
		prior := repository.byKey[reservation.IdempotencyKey]
		prior.Created = false
		return prior, nil
	}
	repository.created++
	run := IntentRunDTO{
		ID: int64(repository.created), Kind: reservation.Task.Kind,
		MonitorID: reservation.Task.MonitorID, DraftID: reservation.Task.DraftID, DraftResourceVersion: reservation.Task.DraftResourceVersion,
		InputHash: reservation.Task.InputHash, Status: "queued", QueuedAt: repository.now,
	}
	result := IntentRunReservationDTO{Run: run, Created: true}
	repository.requestHash[reservation.IdempotencyKey] = reservation.RequestHash
	repository.byKey[reservation.IdempotencyKey] = result
	return result, nil
}

func (repository *intentRunRepositoryFake) FindExpansion(_ context.Context, query ReadExpansionRunQuery) (ExpansionRunDTO, error) {
	if repository.expansion.Run.ID != query.RunID {
		return ExpansionRunDTO{}, ErrIntentRunNotFound
	}
	return cloneExpansionRunDTO(repository.expansion), nil
}

func (repository *intentRunRepositoryFake) FindPreview(_ context.Context, query ReadPreviewRunQuery) (PreviewRunDTO, error) {
	if repository.preview.Run.ID != query.RunID {
		return PreviewRunDTO{}, ErrIntentRunNotFound
	}
	return clonePreviewRunDTO(repository.preview), nil
}

func (repository *intentRunRepositoryFake) SaveTransition(_ context.Context, transition IntentRunTransitionDTO) (IntentRunTransitionReceiptDTO, error) {
	current := &repository.expansion.Run
	if transition.Expected.Kind == "preview" {
		current = &repository.preview.Run
	}
	if !reflect.DeepEqual(cloneIntentRunDTO(*current), cloneIntentRunDTO(transition.Expected)) {
		if reflect.DeepEqual(cloneIntentRunDTO(*current), cloneIntentRunDTO(transition.Next)) {
			return IntentRunTransitionReceiptDTO{Run: cloneIntentRunDTO(*current), Changed: false}, nil
		}
		return IntentRunTransitionReceiptDTO{}, ErrIntentRunStateConflict
	}
	repository.transitions++
	repository.lastTransition = transition
	*current = cloneIntentRunDTO(transition.Next)
	return IntentRunTransitionReceiptDTO{Run: cloneIntentRunDTO(*current), Changed: true}, nil
}

func (repository *intentRunRepositoryFake) CompleteExpansion(_ context.Context, mutation CompleteExpansionRunMutationDTO) (CompleteExpansionRunReceiptDTO, error) {
	if !reflect.DeepEqual(cloneIntentRunDTO(repository.expansion.Run), cloneIntentRunDTO(mutation.Transition.Expected)) {
		return CompleteExpansionRunReceiptDTO{}, ErrIntentRunStateConflict
	}
	if repository.drafts == nil || repository.drafts.draft.DraftID != mutation.DraftMutation.ExpectedDraftID ||
		repository.drafts.draft.ResourceVersion != mutation.DraftMutation.ExpectedResourceVersion {
		return CompleteExpansionRunReceiptDTO{}, ErrIntentVersionConflict
	}
	repository.expansionCompletions++
	repository.lastExpansionCompletion = mutation
	repository.expansion = ExpansionRunDTO{Run: cloneIntentRunDTO(mutation.Transition.Next), Candidates: cloneExpansionRunDTO(ExpansionRunDTO{Candidates: mutation.Candidates}).Candidates}
	repository.drafts.draft = cloneIntentDraftDTO(mutation.DraftMutation.Next)
	return CompleteExpansionRunReceiptDTO{
		Expansion: cloneExpansionRunDTO(repository.expansion), Draft: cloneIntentDraftDTO(repository.drafts.draft),
		ResultFingerprint: mutation.ResultFingerprint, Changed: true,
	}, nil
}

func (repository *intentRunRepositoryFake) CompletePreview(_ context.Context, mutation CompletePreviewRunMutationDTO) (CompletePreviewRunReceiptDTO, error) {
	if !reflect.DeepEqual(cloneIntentRunDTO(repository.preview.Run), cloneIntentRunDTO(mutation.Transition.Expected)) {
		return CompletePreviewRunReceiptDTO{}, ErrIntentRunStateConflict
	}
	repository.previewCompletions++
	repository.lastPreviewCompletion = mutation
	repository.preview = PreviewRunDTO{Run: cloneIntentRunDTO(mutation.Transition.Next), Preview: clonePreviewRunDTO(PreviewRunDTO{Preview: &mutation.Preview}).Preview}
	return CompletePreviewRunReceiptDTO{
		Preview: clonePreviewRunDTO(repository.preview), ResultFingerprint: mutation.ResultFingerprint, Changed: true,
	}, nil
}
