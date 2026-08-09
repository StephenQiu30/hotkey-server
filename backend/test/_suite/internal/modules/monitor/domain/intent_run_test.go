package domain

import (
	"strings"
	"testing"
	"time"
)

func TestIntentAnalysisRunUsesStrictAsyncStateMachine(t *testing.T) {
	t.Parallel()

	queuedAt := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	run, err := NewIntentAnalysisRun(1, IntentRunExpansion, 7, 30, 3, strings.Repeat("a", 64), queuedAt)
	if err != nil {
		t.Fatalf("NewIntentAnalysisRun(): %v", err)
	}
	if _, err := run.Succeed(queuedAt.Add(time.Second)); err == nil {
		t.Fatal("queued run succeeded without starting")
	}
	running, err := run.Start(queuedAt.Add(time.Second))
	if err != nil {
		t.Fatalf("Start(): %v", err)
	}
	succeeded, err := running.Succeed(queuedAt.Add(2 * time.Second))
	if err != nil {
		t.Fatalf("Succeed(): %v", err)
	}
	if succeeded.Status() != IntentRunSucceeded || !succeeded.UsableForDraft(30, 3) {
		t.Fatalf("succeeded run = status %s usable %v", succeeded.Status(), succeeded.UsableForDraft(30, 3))
	}
	if _, err := succeeded.Fail("late failure", queuedAt.Add(3*time.Second)); err == nil {
		t.Fatal("terminal succeeded run was overwritten")
	}
}

func TestIntentAnalysisRunIsInvalidatedWhenDraftChanges(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	run, _ := NewIntentAnalysisRun(2, IntentRunPreview, 7, 30, 3, strings.Repeat("b", 64), now)
	unchanged, invalidated, err := run.InvalidateForDraft(30, 3, now.Add(time.Second))
	if err != nil || invalidated || unchanged.Status() != IntentRunQueued {
		t.Fatalf("same-version invalidation = %#v/%v/%v", unchanged, invalidated, err)
	}
	stale, invalidated, err := run.InvalidateForDraft(30, 4, now.Add(time.Second))
	if err != nil {
		t.Fatalf("InvalidateForDraft(): %v", err)
	}
	if !invalidated || stale.Status() != IntentRunInvalidated || stale.UsableForDraft(30, 4) {
		t.Fatalf("stale run = invalidated %v status %s usable %v", invalidated, stale.Status(), stale.UsableForDraft(30, 4))
	}
	if stale.InvalidatedAt() == nil || !stale.InvalidatedAt().Equal(now.Add(time.Second)) {
		t.Fatalf("stale invalidation time = %#v", stale.InvalidatedAt())
	}
}

func TestIntentAnalysisRunInvalidationPreservesPriorCompletionTimeline(t *testing.T) {
	t.Parallel()

	queuedAt := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	startedAt := queuedAt.Add(time.Second)
	completedAt := queuedAt.Add(2 * time.Second)
	invalidatedAt := queuedAt.Add(3 * time.Second)
	run, _ := NewIntentAnalysisRun(3, IntentRunExpansion, 7, 30, 3, strings.Repeat("c", 64), queuedAt)
	run, _ = run.Start(startedAt)
	run, _ = run.Succeed(completedAt)
	stale, invalidated, err := run.InvalidateForDraft(30, 4, invalidatedAt)
	if err != nil || !invalidated {
		t.Fatalf("InvalidateForDraft(): invalidated=%v error=%v", invalidated, err)
	}
	if stale.CompletedAt() == nil || !stale.CompletedAt().Equal(completedAt) {
		t.Fatalf("prior completion was lost: %#v", stale.CompletedAt())
	}
	if stale.InvalidatedAt() == nil || !stale.InvalidatedAt().Equal(invalidatedAt) {
		t.Fatalf("invalidation time = %#v", stale.InvalidatedAt())
	}
}

func TestIntentAnalysisRunCannotMatchNewDraftWithReusedResourceVersion(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	run, _ := NewIntentAnalysisRun(4, IntentRunPreview, 7, 30, 1, strings.Repeat("d", 64), now)
	if run.UsableForDraft(31, 1) {
		t.Fatal("old run matched a new draft that reused resource version 1")
	}
	invalidated, changed, err := run.InvalidateForDraft(31, 1, now.Add(time.Second))
	if err != nil || !changed || invalidated.Status() != IntentRunInvalidated {
		t.Fatalf("new draft identity invalidation = %#v/%v/%v", invalidated, changed, err)
	}
}
