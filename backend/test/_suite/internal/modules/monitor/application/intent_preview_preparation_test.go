package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIntentServicePreparesPreviewFromExactDurableRunAndImmutableRevision(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 19, 0, 0, 0, time.UTC)
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
	submitted, err := service.SubmitPreviewRun(context.Background(), SubmitPreviewRunCommand{
		MonitorID: 7, DraftID: 101, ExpectedResourceVersion: 4,
		IdempotencyKey: "prepare-preview", EvaluatorProfile: "hybrid-preview-v1", SampleLimit: 25,
	})
	if err != nil {
		t.Fatalf("SubmitPreviewRun(): %v", err)
	}
	tasks.task = IntentAnalysisTaskDTO{
		Run: intentRunReferenceFixture(submitted.Run), AnalysisProfile: "hybrid-preview-v1", SampleLimit: 25,
	}

	prepared, err := service.PrepareIntentPreview(context.Background(), PrepareIntentPreviewQuery{Task: tasks.task})
	if err != nil {
		t.Fatalf("PrepareIntentPreview(): %v", err)
	}
	if prepared.Preview.Task != tasks.task || prepared.Preview.Draft.ResourceVersion != 4 || prepared.Preview.Draft.Objective != draft.Objective {
		t.Fatalf("prepared preview = %#v", prepared.Preview)
	}
	if tasks.reads != 1 || revisions.query.MonitorID != 7 || revisions.query.DraftID != 101 || revisions.query.ResourceVersion != 4 {
		t.Fatalf("durable reads = %d revision query %#v", tasks.reads, revisions.query)
	}

	forged := tasks.task
	forged.SampleLimit++
	if _, err := service.PrepareIntentPreview(context.Background(), PrepareIntentPreviewQuery{Task: forged}); !errors.Is(err, ErrInvalidIntentContract) {
		t.Fatalf("forged preview task error = %v", err)
	}
}
