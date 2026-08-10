package domain

import "testing"

func TestEvidenceStateUsesIndependentLineageRootsWithoutTruthScore(t *testing.T) {
	tests := []struct {
		name  string
		items []EvidenceStateItem
		want  EvidenceState
		count int
	}{
		{name: "no body", want: EvidenceNoCitableBody},
		{name: "single origin", items: []EvidenceStateItem{{1, 11, EvidenceAsserts, true}}, want: EvidenceSingleOrigin, count: 1},
		{name: "duplicate documents remain one origin", items: []EvidenceStateItem{{1, 11, EvidenceAsserts, true}, {2, 11, EvidenceAsserts, true}}, want: EvidenceSingleOrigin, count: 1},
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
