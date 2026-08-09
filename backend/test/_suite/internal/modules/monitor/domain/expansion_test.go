package domain

import (
	"strings"
	"testing"
	"time"
)

func TestLLMExpansionCandidateRequiresAuditableProvenance(t *testing.T) {
	t.Parallel()

	inputHash := strings.Repeat("a", 64)
	if _, err := NewExpansionProvenance(ExpansionSourceLLM, "semantic neighbor", "", "intent-expansion-v1", inputHash); err == nil {
		t.Fatal("LLM candidate without model version was accepted")
	}
	if _, err := NewExpansionProvenance(ExpansionSourceLLM, "semantic neighbor", "model-v1", "", inputHash); err == nil {
		t.Fatal("LLM candidate without prompt version was accepted")
	}
	if _, err := NewExpansionProvenance(ExpansionSourceLLM, "semantic neighbor", "model-v1", "intent-expansion-v1", "not-a-hash"); err == nil {
		t.Fatal("candidate without a valid input hash was accepted")
	}

	provenance := mustExpansionProvenance(t, ExpansionSourceLLM, "semantic neighbor", "model-v1", "intent-expansion-v1", inputHash)
	assessment, err := NewExpansionAssessment(0.86, ExpansionRiskMedium)
	if err != nil {
		t.Fatalf("NewExpansionAssessment(): %v", err)
	}
	candidate, err := NewExpansionCandidate("candidate-1", "企业并购", provenance, assessment)
	if err != nil {
		t.Fatalf("NewExpansionCandidate(): %v", err)
	}
	if candidate.ApprovalStatus() != ExpansionApprovalPending || candidate.Review() != nil {
		t.Fatalf("new candidate approval = %s/%#v", candidate.ApprovalStatus(), candidate.Review())
	}
}

func TestPendingExpansionCannotInfluenceEffectiveTerms(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	pending := mustExpansionCandidate(t, "pending", "acquisition", ExpansionSourceLLM, strings.Repeat("b", 64))
	approvedLLM, err := pending.Decide(ExpansionDecisionApprove, 42, now, "reviewed against examples")
	if err != nil {
		t.Fatalf("Approve LLM candidate: %v", err)
	}
	approvedAlias, err := mustExpansionCandidate(t, "alias", "takeover", ExpansionSourceEntityAlias, strings.Repeat("c", 64)).Decide(ExpansionDecisionApprove, 42, now, "known alias")
	if err != nil {
		t.Fatalf("Approve alias candidate: %v", err)
	}
	rejected, err := mustExpansionCandidate(t, "rejected", "jobs", ExpansionSourceCorpusFeedback, strings.Repeat("d", 64)).Decide(ExpansionDecisionReject, 42, now, "false positive")
	if err != nil {
		t.Fatalf("Reject candidate: %v", err)
	}

	effective := ApprovedExpansionCandidates([]ExpansionCandidate{pending, approvedLLM, rejected, approvedAlias})
	if len(effective) != 2 {
		t.Fatalf("effective candidate count = %d, want 2", len(effective))
	}
	if effective[0].Provenance().Source() != ExpansionSourceEntityAlias || effective[1].Provenance().Source() != ExpansionSourceLLM {
		t.Fatalf("effective priority order = %s, %s", effective[0].Provenance().Source(), effective[1].Provenance().Source())
	}
	if pending.ApprovalStatus() != ExpansionApprovalPending {
		t.Fatal("immutable decision mutated the original candidate")
	}
	if approvedLLM.Review() == nil || approvedLLM.Review().ReviewerUserID() != 42 {
		t.Fatalf("approved review = %#v", approvedLLM.Review())
	}
	if _, err := approvedLLM.Decide(ExpansionDecisionReject, 42, now.Add(time.Minute), "changed mind"); err == nil {
		t.Fatal("terminal candidate decision was overwritten")
	}
}

func TestApprovedExpansionDeduplicatesByFixedSourcePriority(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	llm, _ := mustExpansionCandidate(t, "llm", "M&A", ExpansionSourceLLM, strings.Repeat("e", 64)).Decide(ExpansionDecisionApprove, 7, now, "ok")
	feedback, _ := mustExpansionCandidate(t, "feedback", " m&a ", ExpansionSourceApprovedHistoricalFeedback, strings.Repeat("f", 64)).Decide(ExpansionDecisionApprove, 7, now, "ok")
	effective := ApprovedExpansionCandidates([]ExpansionCandidate{llm, feedback})
	if len(effective) != 1 || effective[0].ID() != "feedback" {
		t.Fatalf("priority winner = %#v, want approved feedback", effective)
	}
}

func TestExpansionSourcePriorityMatchesAcceptedProvenanceOrder(t *testing.T) {
	t.Parallel()

	sources := []ExpansionSource{
		ExpansionSourceUserInput,
		ExpansionSourceEntityAlias,
		ExpansionSourceApprovedHistoricalFeedback,
		ExpansionSourceCorpusFeedback,
		ExpansionSourceLLM,
	}
	for index, source := range sources {
		if got := source.Priority(); got != index {
			t.Fatalf("source %s priority = %d, want %d", source, got, index)
		}
	}
}

func mustExpansionProvenance(t *testing.T, source ExpansionSource, reason, modelVersion, promptVersion, inputHash string) ExpansionProvenance {
	t.Helper()
	provenance, err := NewExpansionProvenance(source, reason, modelVersion, promptVersion, inputHash)
	if err != nil {
		t.Fatalf("NewExpansionProvenance(%s): %v", source, err)
	}
	return provenance
}

func mustExpansionCandidate(t *testing.T, id, value string, source ExpansionSource, inputHash string) ExpansionCandidate {
	t.Helper()
	modelVersion, promptVersion := "", ""
	if source == ExpansionSourceLLM {
		modelVersion, promptVersion = "model-v1", "prompt-v1"
	}
	provenance := mustExpansionProvenance(t, source, "candidate provenance", modelVersion, promptVersion, inputHash)
	assessment, err := NewExpansionAssessment(0.75, ExpansionRiskLow)
	if err != nil {
		t.Fatalf("NewExpansionAssessment(): %v", err)
	}
	candidate, err := NewExpansionCandidate(id, value, provenance, assessment)
	if err != nil {
		t.Fatalf("NewExpansionCandidate(): %v", err)
	}
	return candidate
}
