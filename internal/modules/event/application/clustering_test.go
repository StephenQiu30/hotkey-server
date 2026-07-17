package application

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

func TestClusteringProducesAuditableDeterministicDecisions(t *testing.T) {
	first, second := int64(1), int64(2)
	input := ClusteringInput{
		ContentID: 10, ClusteringVersion: "v1", FeatureInputHash: domain.FeatureInputHash("content", "10"),
		Candidates: []domain.Candidate{
			{EventID: first, EventKey: "evt_a", Channel: domain.ChannelLexical, Score: 90},
			{EventID: second, EventKey: "evt_b", Channel: domain.ChannelTemporal, Score: 80},
		},
		Scores: map[string]domain.ScoreBreakdown{
			"evt_a": {EntityAction: 100, Semantic: 90, Temporal: 80, Location: 70, SourceContext: 60},
			"evt_b": {EntityAction: 60, Semantic: 60, Temporal: 60, Location: 60, SourceContext: 60},
		},
		HardConflicts: map[string]bool{"evt_a": true},
	}
	decisions, err := NewClusteringService().Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(decisions) != 3 || decisions[0].Decision != domain.DecisionReject || decisions[1].Decision != domain.DecisionReject || decisions[2].Decision != domain.DecisionNewEvent {
		t.Fatalf("Evaluate() = %#v, want rejected candidates then new-event decision", decisions)
	}
	for _, decision := range decisions {
		if err := decision.Validate(); err != nil {
			t.Fatalf("decision %#v is not persistable: %v", decision, err)
		}
	}
}

func TestClusteringCreatesNewEventDecisionWhenNoCandidates(t *testing.T) {
	decisions, err := NewClusteringService().Evaluate(context.Background(), ClusteringInput{ContentID: 1, ClusteringVersion: "v1", FeatureInputHash: domain.FeatureInputHash("empty"), Scores: map[string]domain.ScoreBreakdown{}})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != domain.DecisionNewEvent || decisions[0].CandidateEventKey != "__new_event__" {
		t.Fatalf("Evaluate() = %#v, want new-event decision", decisions)
	}
}

func TestClusteringAcceptsOnlyTheHighestScoringCandidate(t *testing.T) {
	first, second := int64(1), int64(2)
	decisions, err := NewClusteringService().Evaluate(context.Background(), ClusteringInput{
		ContentID: 10, ClusteringVersion: "v1", FeatureInputHash: domain.FeatureInputHash("content", "10"),
		Candidates: []domain.Candidate{
			{EventID: first, EventKey: "evt_a", Channel: domain.ChannelLexical, Score: 90},
			{EventID: second, EventKey: "evt_b", Channel: domain.ChannelTemporal, Score: 80},
		},
		Scores: map[string]domain.ScoreBreakdown{
			"evt_a": {EntityAction: 100, Semantic: 100, Temporal: 100, Location: 100, SourceContext: 100},
			"evt_b": {EntityAction: 90, Semantic: 90, Temporal: 90, Location: 90, SourceContext: 90},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	accepted := 0
	for _, decision := range decisions {
		if decision.Decision == domain.DecisionAccept {
			accepted++
			if decision.CandidateEventKey != "evt_a" {
				t.Fatalf("accepted candidate = %q, want evt_a", decision.CandidateEventKey)
			}
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted decisions = %d, want 1: %#v", accepted, decisions)
	}
}

func TestClusteringPersistsCandidateAuditInputs(t *testing.T) {
	candidateID := int64(9)
	decisions, err := NewClusteringService().Evaluate(context.Background(), ClusteringInput{
		ContentID: 10, ClusteringVersion: "v1", FeatureInputHash: domain.FeatureInputHash("content", "audit"),
		Candidates: []domain.Candidate{{
			EventID: candidateID, EventKey: "evt_audit", Channel: domain.ChannelVector, Score: 95,
			RecallSources:      []domain.CandidateRecall{{Channel: domain.ChannelLexical, Score: 82}, {Channel: domain.ChannelVector, Score: 95}},
			EvidenceContentIDs: []int64{7, 8},
		}},
		Scores: map[string]domain.ScoreBreakdown{
			"evt_audit": {EntityAction: 90, Semantic: 95, Temporal: 85, Location: 80, SourceContext: 75},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(decisions) != 1 || decisions[0].Decision != domain.DecisionAccept {
		t.Fatalf("decisions = %#v", decisions)
	}
	decision := decisions[0]
	channels, ok := decision.FeatureSnapshot["recall_channels"].([]string)
	if !ok || len(channels) != 2 || channels[0] != "lexical" || channels[1] != "vector" {
		t.Fatalf("recall channels = %#v", decision.FeatureSnapshot)
	}
	if len(decision.ReasonCodes) != 3 || decision.ReasonCodes[2] != "membership_threshold_accepted" {
		t.Fatalf("reason codes = %#v", decision.ReasonCodes)
	}
	if got := decision.EvidenceContentIDs; len(got) != 3 || got[0] != 7 || got[1] != 8 || got[2] != 10 {
		t.Fatalf("evidence content IDs = %#v", got)
	}
}
