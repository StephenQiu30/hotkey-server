package domain

import (
	"strings"
	"testing"
	"time"
)

func TestIntentDraftReplacementIsVersionedAndInvalidatesCandidates(t *testing.T) {
	t.Parallel()

	firstDefinition := mustIntentDefinition(t, "Track AI acquisitions")
	candidate := mustExpansionCandidate(t, "candidate", "takeover", ExpansionSourceLLM, strings.Repeat("a", 64))
	draft, err := NewIntentDraft(11, 101, 4, firstDefinition, []ExpansionCandidate{candidate})
	if err != nil {
		t.Fatalf("NewIntentDraft(): %v", err)
	}
	secondDefinition := mustIntentDefinition(t, "Track robotics acquisitions")
	if _, err := draft.ReplaceDefinition(3, secondDefinition); err == nil {
		t.Fatal("stale draft replacement was accepted")
	}
	replaced, err := draft.ReplaceDefinition(4, secondDefinition)
	if err != nil {
		t.Fatalf("ReplaceDefinition(): %v", err)
	}
	if replaced.DraftID() != 101 || replaced.ResourceVersion() != 5 || len(replaced.Candidates()) != 0 {
		t.Fatalf("replaced draft = version %d candidates %d", replaced.ResourceVersion(), len(replaced.Candidates()))
	}
	if draft.ResourceVersion() != 4 || len(draft.Candidates()) != 1 {
		t.Fatal("replacement mutated the prior draft snapshot")
	}
}

func TestIntentDraftCandidateReviewCreatesNewVersion(t *testing.T) {
	t.Parallel()

	draft, err := NewIntentDraft(11, 101, 4, mustIntentDefinition(t, "Track AI acquisitions"), []ExpansionCandidate{
		mustExpansionCandidate(t, "candidate", "takeover", ExpansionSourceLLM, strings.Repeat("b", 64)),
	})
	if err != nil {
		t.Fatalf("NewIntentDraft(): %v", err)
	}
	reviewed, err := draft.ReviewCandidate(4, "candidate", ExpansionDecisionApprove, 99, time.Date(2026, 8, 9, 11, 0, 0, 0, time.UTC), "validated")
	if err != nil {
		t.Fatalf("ReviewCandidate(): %v", err)
	}
	if reviewed.ResourceVersion() != 5 || reviewed.Candidates()[0].ApprovalStatus() != ExpansionApprovalApproved {
		t.Fatalf("reviewed draft = version %d status %s", reviewed.ResourceVersion(), reviewed.Candidates()[0].ApprovalStatus())
	}
	if draft.Candidates()[0].ApprovalStatus() != ExpansionApprovalPending {
		t.Fatal("candidate review mutated the previous draft")
	}
	withoutCandidate, _ := NewIntentDraft(11, 101, 4, draft.Definition(), nil)
	if draft.MatchingFingerprint() != withoutCandidate.MatchingFingerprint() {
		t.Fatal("pending candidate changed matching fingerprint")
	}
	if reviewed.MatchingFingerprint() == draft.MatchingFingerprint() {
		t.Fatal("approved candidate did not change matching fingerprint")
	}
}

func TestPublishedIntentVersionKeepsImmutableApprovedSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	approved, _ := mustExpansionCandidate(t, "candidate", "takeover", ExpansionSourceLLM, strings.Repeat("c", 64)).Decide(ExpansionDecisionApprove, 9, now.Add(-time.Minute), "approved")
	pending := mustExpansionCandidate(t, "pending", "merger", ExpansionSourceLLM, strings.Repeat("e", 64))
	draft, _ := NewIntentDraft(11, 101, 7, mustIntentDefinition(t, "Track AI acquisitions"), []ExpansionCandidate{approved, pending})
	version, err := NewPublishedIntentVersion(3, draft, now)
	if err != nil {
		t.Fatalf("NewPublishedIntentVersion(): %v", err)
	}
	if version.ID() != 101 || version.MonitorID() != 11 || version.Revision() != 3 || version.SourceDraftResourceVersion() != 7 {
		t.Fatalf("published identity = %d/%d/%d/%d", version.ID(), version.MonitorID(), version.Revision(), version.SourceDraftResourceVersion())
	}
	candidates := version.ApprovedCandidates()
	if len(version.Candidates()) != 2 || len(candidates) != 1 {
		t.Fatalf("published candidate audit/effective counts = %d/%d", len(version.Candidates()), len(candidates))
	}
	candidates[0] = mustExpansionCandidate(t, "other", "other", ExpansionSourceCorpusFeedback, strings.Repeat("d", 64))
	if version.ApprovedCandidates()[0].ID() != "candidate" {
		t.Fatal("published version candidates were mutated through accessor")
	}
}

func TestIntentDraftRejectsExpansionFromAnotherDraftIdentity(t *testing.T) {
	t.Parallel()

	draft, _ := NewIntentDraft(11, 101, 4, mustIntentDefinition(t, "Track AI acquisitions"), nil)
	candidate := mustExpansionCandidate(t, "candidate", "takeover", ExpansionSourceLLM, strings.Repeat("f", 64))
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	matchingRun := mustSucceededIntentRun(t, IntentRunExpansion, 11, 101, 4, strings.Repeat("f", 64), now)

	otherDraftRun := mustSucceededIntentRun(t, IntentRunExpansion, 11, 102, 4, strings.Repeat("f", 64), now)
	if _, err := draft.AttachExpansionCandidates(4, otherDraftRun, []ExpansionCandidate{candidate}); err == nil {
		t.Fatal("expansion from another draft identity was attached")
	}
	otherMonitorRun := mustSucceededIntentRun(t, IntentRunExpansion, 12, 101, 4, strings.Repeat("f", 64), now)
	if _, err := draft.AttachExpansionCandidates(4, otherMonitorRun, []ExpansionCandidate{candidate}); err == nil {
		t.Fatal("expansion from another monitor was attached")
	}
	previewRun := mustSucceededIntentRun(t, IntentRunPreview, 11, 101, 4, strings.Repeat("f", 64), now)
	if _, err := draft.AttachExpansionCandidates(4, previewRun, []ExpansionCandidate{candidate}); err == nil {
		t.Fatal("preview output was attached as expansion candidates")
	}
	queuedRun, _ := NewIntentAnalysisRun(99, IntentRunExpansion, 11, 101, 4, strings.Repeat("f", 64), now)
	if _, err := draft.AttachExpansionCandidates(4, queuedRun, []ExpansionCandidate{candidate}); err == nil {
		t.Fatal("unfinished expansion output was attached")
	}
	wrongInputCandidate := mustExpansionCandidate(t, "wrong-input", "merger", ExpansionSourceLLM, strings.Repeat("a", 64))
	if _, err := draft.AttachExpansionCandidates(4, matchingRun, []ExpansionCandidate{wrongInputCandidate}); err == nil {
		t.Fatal("candidate with another input hash was attached")
	}
	attached, err := draft.AttachExpansionCandidates(4, matchingRun, []ExpansionCandidate{candidate})
	if err != nil {
		t.Fatalf("AttachExpansionCandidates(): %v", err)
	}
	if attached.DraftID() != 101 || attached.ResourceVersion() != 5 || len(attached.Candidates()) != 1 {
		t.Fatalf("attached draft = id %d version %d candidates %d", attached.DraftID(), attached.ResourceVersion(), len(attached.Candidates()))
	}
}

func mustSucceededIntentRun(t *testing.T, kind IntentRunKind, monitorID, draftID, resourceVersion int64, inputHash string, queuedAt time.Time) IntentAnalysisRun {
	t.Helper()
	run, err := NewIntentAnalysisRun(91, kind, monitorID, draftID, resourceVersion, inputHash, queuedAt)
	if err != nil {
		t.Fatalf("NewIntentAnalysisRun(): %v", err)
	}
	run, err = run.Start(queuedAt.Add(time.Second))
	if err != nil {
		t.Fatalf("Start(): %v", err)
	}
	run, err = run.Succeed(queuedAt.Add(2 * time.Second))
	if err != nil {
		t.Fatalf("Succeed(): %v", err)
	}
	return run
}

func mustIntentDefinition(t *testing.T, objectiveText string) IntentDefinition {
	t.Helper()
	objective, err := NewIntentObjective(objectiveText)
	if err != nil {
		t.Fatalf("NewIntentObjective(): %v", err)
	}
	definition, err := NewIntentDefinition(objective, []IntentClause{mustIntentClause(t, IntentClauseMust, IntentClauseAction, "acquisition")}, nil, nil)
	if err != nil {
		t.Fatalf("NewIntentDefinition(): %v", err)
	}
	return definition
}
