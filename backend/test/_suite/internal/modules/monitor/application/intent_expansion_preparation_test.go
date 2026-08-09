package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIntentServicePreparesExpansionFromExactDurableRunAndImmutableRevision(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 18, 0, 0, 0, time.UTC)
	draft := intentDraftFixture()
	drafts := &intentDraftRepositoryFake{draft: draft}
	runs := newIntentRunRepositoryFake(now)
	tasks := &intentAnalysisTaskRepositoryFake{}
	revisions := &intentDraftRevisionRepositoryFake{draft: draft}
	service, err := NewIntentService(IntentServiceDependencies{
		Drafts: drafts, Runs: runs, Tasks: tasks, Revisions: revisions, Clock: fixedIntentClock{now: now},
	})
	if err != nil {
		t.Fatalf("NewIntentService(): %v", err)
	}
	submitted, err := service.SubmitExpansionRun(context.Background(), SubmitExpansionRunCommand{
		MonitorID: 7, DraftID: 101, ExpectedResourceVersion: 4,
		IdempotencyKey: "prepare-expansion", ExpansionProfile: "monitor-intent-expansion-v1",
	})
	if err != nil {
		t.Fatalf("SubmitExpansionRun(): %v", err)
	}
	tasks.task = IntentAnalysisTaskDTO{
		Run: intentRunReferenceFixture(submitted.Run), AnalysisProfile: "monitor-intent-expansion-v1",
	}

	resolved, err := service.ReadIntentAnalysisTask(context.Background(), ReadIntentAnalysisTaskQuery{
		RunID: submitted.Run.ID, DraftID: 101, DraftResourceVersion: 4,
	})
	if err != nil {
		t.Fatalf("ReadIntentAnalysisTask(): %v", err)
	}
	prepared, err := service.PrepareIntentExpansion(context.Background(), PrepareIntentExpansionQuery{Task: resolved.Task})
	if err != nil {
		t.Fatalf("PrepareIntentExpansion(): %v", err)
	}
	if prepared.Expansion.Task.Run.InputHash != submitted.Run.InputHash || prepared.Expansion.Draft.ResourceVersion != 4 || prepared.Expansion.Draft.Objective != draft.Objective {
		t.Fatalf("prepared expansion = %#v", prepared.Expansion)
	}
	if tasks.reads != 2 || revisions.query.MonitorID != 7 || revisions.query.DraftID != 101 || revisions.query.ResourceVersion != 4 {
		t.Fatalf("durable reads = %d revision query %#v", tasks.reads, revisions.query)
	}

	forged := resolved.Task
	forged.AnalysisProfile = "different-profile"
	if _, err := service.PrepareIntentExpansion(context.Background(), PrepareIntentExpansionQuery{Task: forged}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("forged task preparation error = %v", err)
	}
	forged = resolved.Task
	forged.Run.InputHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if forged.Run.InputHash == resolved.Task.Run.InputHash {
		forged.Run.InputHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	}
	if _, err := service.PrepareIntentExpansion(context.Background(), PrepareIntentExpansionQuery{Task: forged}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("forged input hash preparation error = %v", err)
	}
}

func TestIntentServiceRejectsTaskOrRevisionIdentityDrift(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 18, 30, 0, 0, time.UTC)
	task := IntentAnalysisTaskDTO{
		Run: IntentRunReferenceDTO{
			RunID: 81, Kind: "expansion", MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
			InputHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		AnalysisProfile: "monitor-intent-expansion-v1",
	}
	tasks := &intentAnalysisTaskRepositoryFake{task: task}
	revision := intentDraftFixture()
	revision.MonitorID = 8
	service, err := NewIntentService(IntentServiceDependencies{
		Drafts: &intentDraftRepositoryFake{draft: intentDraftFixture()}, Runs: newIntentRunRepositoryFake(now),
		Tasks: tasks, Revisions: &intentDraftRevisionRepositoryFake{draft: revision}, Clock: fixedIntentClock{now: now},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadIntentAnalysisTask(context.Background(), ReadIntentAnalysisTaskQuery{RunID: 81, DraftID: 102, DraftResourceVersion: 4}); !errors.Is(err, ErrIntentRunNotFound) {
		t.Fatalf("cross-draft task read error = %v", err)
	}
	if _, err := service.PrepareIntentExpansion(context.Background(), PrepareIntentExpansionQuery{Task: task}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("cross-monitor revision preparation error = %v", err)
	}
}

type intentAnalysisTaskRepositoryFake struct {
	task  IntentAnalysisTaskDTO
	reads int
}

func (repository *intentAnalysisTaskRepositoryFake) FindIntentAnalysisTask(_ context.Context, query ReadIntentAnalysisTaskQuery) (IntentAnalysisTaskDTO, error) {
	repository.reads++
	if repository.task.Run.RunID != query.RunID || repository.task.Run.DraftID != query.DraftID || repository.task.Run.DraftResourceVersion != query.DraftResourceVersion {
		return IntentAnalysisTaskDTO{}, ErrIntentRunNotFound
	}
	return repository.task, nil
}

type intentDraftRevisionRepositoryFake struct {
	draft IntentDraftDTO
	query ReadIntentDraftRevisionQuery
}

func (repository *intentDraftRevisionRepositoryFake) FindIntentDraftRevision(_ context.Context, query ReadIntentDraftRevisionQuery) (IntentDraftDTO, error) {
	repository.query = query
	return cloneIntentDraftDTO(repository.draft), nil
}
