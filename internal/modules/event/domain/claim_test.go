package domain

import "testing"

func TestEventSummaryRequiresActiveEvidence(t *testing.T) {
	summary := EventSummary{Version: "v1", TitleZH: "事件", Sentences: []EvidenceSentence{{Text: "事实", Evidence: []EvidenceRef{{ContentID: 7, Locator: "title"}}}}}
	if err := summary.Validate(map[int64]bool{7: true}); err != nil {
		t.Fatal(err)
	}
	if err := summary.Validate(map[int64]bool{}); err == nil {
		t.Fatal("inactive evidence accepted")
	}
}

func TestClaimValidationRejectsUnknownStatus(t *testing.T) {
	claim := Claim{ID: 1, Version: 1, EventID: 1, NormalizedClaim: "claim", ClaimHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: ClaimStatus("unknown")}
	if err := claim.Validate(); err == nil {
		t.Fatal("unknown status accepted")
	}
}
