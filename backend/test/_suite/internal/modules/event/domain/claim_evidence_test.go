package domain

import (
	"encoding/json"
	"slices"
	"testing"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

func TestEvidenceStateUsesIndependentLineageRootsWithoutTruthScore(t *testing.T) {
	tests := []struct {
		name  string
		items []EvidenceStateItem
		want  EvidenceState
		count int
	}{
		{name: "no body", want: EvidenceNoCitableBody},
		{name: "single origin", items: []EvidenceStateItem{{1, 11, EvidenceAsserts, true}}, want: EvidenceSingleOrigin, count: 1},
		{name: "comments and reposts from one origin remain one independent source", items: []EvidenceStateItem{
			{1, 11, EvidenceAsserts, true}, {2, 11, EvidenceMentions, true}, {3, 11, EvidenceAttributes, true},
		}, want: EvidenceSingleOrigin, count: 1},
		{name: "multiple origins", items: []EvidenceStateItem{{1, 11, EvidenceAsserts, true}, {2, 12, EvidenceAttributes, true}}, want: EvidenceMultipleOrigins, count: 2},
		{name: "conflict", items: []EvidenceStateItem{{1, 11, EvidenceAsserts, true}, {2, 12, EvidenceContradicts, true}}, want: EvidenceConflictingReports, count: 2},
		{name: "correction precedence", items: []EvidenceStateItem{{1, 11, EvidenceContradicts, true}, {2, 11, EvidenceCorrects, true}}, want: EvidencePublisherCorrected, count: 1},
		{name: "withdrawal precedence", items: []EvidenceStateItem{{1, 11, EvidenceCorrects, true}, {2, 11, EvidenceWithdraws, true}}, want: EvidencePublisherWithdrawn, count: 1},
		{name: "expired ignored", items: []EvidenceStateItem{{1, 11, EvidenceAsserts, false}}, want: EvidenceNoCitableBody},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CalculateEvidenceState(EvidenceStateInput{AlgorithmVersion: "evidence-state-lineage-v2", Items: test.items})
			if err != nil || got.State != test.want || got.IndependentOriginCount != test.count || len(got.ReasonCodes) != 1 {
				t.Fatalf("CalculateEvidenceState() = %#v, %v", got, err)
			}
		})
	}
}

func TestEvidenceStateRejectsUnknownRelation(t *testing.T) {
	if _, err := CalculateEvidenceState(EvidenceStateInput{AlgorithmVersion: "v2", Items: []EvidenceStateItem{{1, 1, "supports", true}}}); err == nil {
		t.Fatal("legacy supports relation was accepted")
	}
}

func TestRelevanceHeatClaimEvidenceAndEventEvidenceStateRemainIndependent(t *testing.T) {
	t.Parallel()
	relevance := ingestiondomain.RelevanceSnapshotInput{MonitorID: 1, MonitorConfigVersionID: 2, ContentID: 3,
		InputHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ScoringVersion: "relevance-v2",
		RecallPaths: []string{"source"}, RuleScore: 92, FinalScore: 92,
		Decision: ingestiondomain.MatchDecisionAccepted, DecisionOrigin: ingestiondomain.DecisionOriginRule,
		Explanation: json.RawMessage(`{}`)}
	if err := relevance.Validate(); err != nil {
		t.Fatalf("relevance.Validate() error = %v", err)
	}
	heatInput := EventHeatInput{IndependentLineageRoots: 2, ReportsInWindow: 4, ReportsInPreviousWindow: 2,
		ReportsInPriorWindow: 1, PublisherCoverage: 2, SourceTypeCoverage: 2, TemporalBaselineAvailable: true,
		AgeHours: 3, ProfileVersion: "heat-v2"}
	heat, err := CalculateEventHeat(heatInput)
	if err != nil {
		t.Fatal(err)
	}
	claimEvidence := []EvidenceStateItem{{ClaimEvidenceVersionID: 101, LineageRootID: 10, Relation: EvidenceAsserts, Citable: true}}
	evidenceState, err := CalculateEvidenceState(EvidenceStateInput{AlgorithmVersion: "evidence-state-lineage-v2", Items: claimEvidence})
	if err != nil {
		t.Fatal(err)
	}

	changedRelevance := relevance
	changedRelevance.RuleScore, changedRelevance.FinalScore = 8, 8
	heatAfterRelevanceChange, heatErr := CalculateEventHeat(heatInput)
	evidenceAfterRelevanceChange, evidenceErr := CalculateEvidenceState(EvidenceStateInput{
		AlgorithmVersion: "evidence-state-lineage-v2", Items: claimEvidence,
	})
	if err := changedRelevance.Validate(); err != nil || heatErr != nil || evidenceErr != nil ||
		heatAfterRelevanceChange.Score != heat.Score || evidenceAfterRelevanceChange.State != evidenceState.State ||
		evidenceAfterRelevanceChange.IndependentOriginCount != evidenceState.IndependentOriginCount ||
		!slices.Equal(evidenceAfterRelevanceChange.ReasonCodes, evidenceState.ReasonCodes) ||
		claimEvidence[0].ClaimEvidenceVersionID != 101 {
		t.Fatalf("relevance change crossed dimensions: relevance=%#v heat=%#v evidence=%#v error=%v",
			changedRelevance, heatAfterRelevanceChange, evidenceAfterRelevanceChange, err)
	}

	changedHeatInput := heatInput
	changedHeatInput.ReportsInWindow = 10
	changedHeat, err := CalculateEventHeat(changedHeatInput)
	evidenceAfterHeatChange, evidenceErr := CalculateEvidenceState(EvidenceStateInput{
		AlgorithmVersion: "evidence-state-lineage-v2", Items: claimEvidence,
	})
	if err != nil || evidenceErr != nil || changedHeat.Score == heat.Score || changedRelevance.FinalScore != 8 ||
		evidenceAfterHeatChange.State != evidenceState.State ||
		evidenceAfterHeatChange.IndependentOriginCount != evidenceState.IndependentOriginCount ||
		!slices.Equal(evidenceAfterHeatChange.ReasonCodes, evidenceState.ReasonCodes) {
		t.Fatalf("heat change crossed dimensions: relevance=%#v heat=%#v evidence=%#v error=%v",
			changedRelevance, changedHeat, evidenceState, err)
	}

	changedClaimEvidence := append([]EvidenceStateItem(nil), claimEvidence...)
	changedClaimEvidence[0].Relation = EvidenceContradicts
	changedEvidenceState, err := CalculateEvidenceState(EvidenceStateInput{
		AlgorithmVersion: "evidence-state-lineage-v2", Items: changedClaimEvidence,
	})
	heatAfterEvidenceChange, heatErr := CalculateEventHeat(changedHeatInput)
	if err != nil || heatErr != nil || changedEvidenceState.State != EvidenceConflictingReports ||
		heatAfterEvidenceChange.Score != changedHeat.Score || changedRelevance.FinalScore != 8 {
		t.Fatalf("claim evidence change crossed dimensions: relevance=%#v heat=%#v evidence=%#v error=%v",
			changedRelevance, changedHeat, changedEvidenceState, err)
	}
}
