package domain

import "testing"

func TestEvidenceSummaryRequiresCitationOrEditorialNote(t *testing.T) {
	actorID, modelRunID := int64(1), int64(2)
	valid := []EvidenceSummarySentence{
		{Text: "A cited report sentence.", EvidenceIDs: []int64{10}, DecisionOrigin: "automatic", ModelRunID: &modelRunID},
		{Text: "Editor's context.", EditorialNote: true, DecisionOrigin: "manual", ActorUserID: &actorID},
	}
	if err := ValidateEvidenceSummarySentences(valid); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]EvidenceSummarySentence{
		{{Text: "uncited", DecisionOrigin: "manual", ActorUserID: &actorID}},
		{{Text: "editorial with citation", EditorialNote: true, EvidenceIDs: []int64{1}, DecisionOrigin: "manual", ActorUserID: &actorID}},
		{{Text: "model editorial", EditorialNote: true, DecisionOrigin: "automatic", ModelRunID: &modelRunID}},
	} {
		if err := ValidateEvidenceSummarySentences(invalid); err == nil {
			t.Fatalf("invalid summary accepted: %#v", invalid)
		}
	}
}
