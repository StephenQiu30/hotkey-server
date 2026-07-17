package domain

import (
	"math"
	"testing"
	"time"
)

func TestLifecycleStateMachineUsesDetectedAsInitialState(t *testing.T) {
	allowed := [][2]LifecycleStatus{
		{LifecycleDetected, LifecycleActive},
		{LifecycleActive, LifecycleCooling},
		{LifecycleCooling, LifecycleActive},
		{LifecycleClosed, LifecycleActive},
		{LifecycleActive, LifecycleMerged},
	}
	for _, transition := range allowed {
		if !CanTransition(transition[0], transition[1]) {
			t.Fatalf("CanTransition(%q, %q) = false", transition[0], transition[1])
		}
	}
	for _, transition := range [][2]LifecycleStatus{
		{LifecycleArchived, LifecycleActive},
		{LifecycleRejected, LifecycleActive},
		{LifecycleMerged, LifecycleActive},
		{LifecycleDetected, LifecycleArchived},
	} {
		if CanTransition(transition[0], transition[1]) {
			t.Fatalf("CanTransition(%q, %q) accepted illegal transition", transition[0], transition[1])
		}
	}
}

func TestDecideUsesBoundedWeightedScoresAndHardConflict(t *testing.T) {
	scores := ScoreBreakdown{EntityAction: 100, Semantic: 100, Temporal: 100, Location: 100, SourceContext: 100}
	decision, value, err := Decide(scores, false)
	if err != nil || decision != DecisionAccept || value != 100 {
		t.Fatalf("Decide() = %q/%v/%v, want accepted/100/nil", decision, value, err)
	}
	decision, _, err = Decide(scores, true)
	if err != nil || decision != DecisionReject {
		t.Fatalf("hard-conflict Decide() = %q/%v, want rejected", decision, err)
	}
	for _, scores := range []ScoreBreakdown{{EntityAction: 64}, {EntityAction: 65}, {EntityAction: 80}} {
		if _, _, err := Decide(scores, false); err != nil {
			t.Fatalf("Decide(%#v) error = %v", scores, err)
		}
	}
	if _, _, err := Decide(ScoreBreakdown{Semantic: math.NaN()}, false); err == nil {
		t.Fatal("Decide() accepted NaN")
	}
}

func TestCompareCandidatesCapsAndUsesStableEventKeyTieBreak(t *testing.T) {
	candidates := make([]Candidate, 0, MaxCandidates+2)
	for i := 0; i < MaxCandidates+2; i++ {
		candidates = append(candidates, Candidate{EventID: int64(i + 1), EventKey: string(rune('z' - i%26)), Channel: ChannelLexical, Score: 50})
	}
	got := CompareCandidates(candidates)
	if len(got) != MaxCandidates {
		t.Fatalf("CompareCandidates() length = %d, want %d", len(got), MaxCandidates)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Score < got[i].Score || got[i-1].Score == got[i].Score && got[i-1].EventKey > got[i].EventKey {
			t.Fatalf("candidates not sorted deterministically at %d: %#v %#v", i, got[i-1], got[i])
		}
	}
}

func TestDecisionIdempotencyAndAuditValidation(t *testing.T) {
	id := int64(7)
	decision := Decision{ContentID: 10, CandidateEventID: &id, CandidateEventKey: "evt_7", ClusteringVersion: "v1", FeatureInputHash: FeatureInputHash("content", "10"), Channel: ChannelFingerprint, CandidateRank: 1, Scores: ScoreBreakdown{EntityAction: 80}, MembershipScore: 24, Decision: DecisionAccept, DecisionOrigin: DecisionOriginRule}
	if err := decision.Validate(); err != nil {
		t.Fatalf("Decision.Validate() error = %v", err)
	}
	if decision.IdempotencyKey() != decision.IdempotencyKey() {
		t.Fatal("IdempotencyKey() is not stable")
	}
	created := time.Now().UTC()
	targetID := int64(8)
	audit := GovernanceAudit{EventID: 7, Action: AuditMerge, ReasonCode: "manual_confirmed", SourceEventID: &id, TargetEventID: &targetID, CreatedAt: created}
	if err := audit.Validate(); err != nil {
		t.Fatalf("GovernanceAudit.Validate() error = %v", err)
	}
}
