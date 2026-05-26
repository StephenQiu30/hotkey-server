package trust

import "testing"

func TestEvidenceSeparatesFactAndSignalTrust(t *testing.T) {
	service := NewService()

	if err := service.AddEvidence(EvidenceInput{
		EventID:      "cluster_1",
		SourceID:     "arxiv-ai",
		SourceItemID: "item_1",
		Layer:        EvidenceLayerFact,
		TrustLevel:   TrustLevelHigh,
		Title:        "Research paper confirms the model release",
		URL:          "https://arxiv.org/abs/2605.00001",
	}); err != nil {
		t.Fatalf("AddEvidence fact returned error: %v", err)
	}
	if err := service.AddEvidence(EvidenceInput{
		EventID:      "cluster_1",
		SourceID:     "low-trust-social",
		SourceItemID: "item_2",
		Layer:        EvidenceLayerSignal,
		TrustLevel:   TrustLevelLow,
		Title:        "A viral repost about the release",
		URL:          "https://example.com/repost",
		HeatWeight:   7,
	}); err != nil {
		t.Fatalf("AddEvidence signal returned error: %v", err)
	}

	detail := service.GetEventEvidence("cluster_1")
	if len(detail.FactEvidence) != 1 {
		t.Fatalf("fact evidence len = %d, want 1", len(detail.FactEvidence))
	}
	if len(detail.SignalEvidence) != 1 {
		t.Fatalf("signal evidence len = %d, want 1", len(detail.SignalEvidence))
	}
	if detail.FactScore != 100 {
		t.Fatalf("factScore = %d, want 100", detail.FactScore)
	}
	if detail.HeatScore != 7 {
		t.Fatalf("heatScore = %d, want 7", detail.HeatScore)
	}
	if detail.TrustLabel != TrustLabelVerified {
		t.Fatalf("trustLabel = %q, want %q", detail.TrustLabel, TrustLabelVerified)
	}
}

func TestLowTrustSignalDoesNotCreateFactEvidence(t *testing.T) {
	service := NewService()

	if err := service.AddEvidence(EvidenceInput{
		EventID:      "cluster_1",
		SourceID:     "low-trust-social",
		SourceItemID: "item_1",
		Layer:        EvidenceLayerSignal,
		TrustLevel:   TrustLevelLow,
		Title:        "Viral but unverified repost",
		URL:          "https://example.com/repost",
		HeatWeight:   3,
	}); err != nil {
		t.Fatalf("AddEvidence returned error: %v", err)
	}

	detail := service.GetEventEvidence("cluster_1")
	if len(detail.FactEvidence) != 0 {
		t.Fatalf("low trust signal created fact evidence: %#v", detail.FactEvidence)
	}
	if detail.FactScore != 0 {
		t.Fatalf("factScore = %d, want 0", detail.FactScore)
	}
	if detail.HeatScore != 3 {
		t.Fatalf("heatScore = %d, want 3", detail.HeatScore)
	}
	if detail.TrustLabel != TrustLabelUnverified {
		t.Fatalf("trustLabel = %q, want %q", detail.TrustLabel, TrustLabelUnverified)
	}
}

func TestAISummaryRequiresSourceCitations(t *testing.T) {
	service := NewService()

	err := service.SetAISummary("cluster_1", AISummaryInput{
		Summary: "OpenAI released a new model.",
	})
	if err != ErrMissingCitation {
		t.Fatalf("err = %v, want %v", err, ErrMissingCitation)
	}

	if err := service.SetAISummary("cluster_1", AISummaryInput{
		Summary:     "OpenAI released a new model.",
		CitationIDs: []string{"item_1"},
	}); err != nil {
		t.Fatalf("SetAISummary returned error: %v", err)
	}

	detail := service.GetEventEvidence("cluster_1")
	if detail.AISummary == nil {
		t.Fatalf("aiSummary is nil")
	}
	if len(detail.AISummary.CitationIDs) != 1 || detail.AISummary.CitationIDs[0] != "item_1" {
		t.Fatalf("citation ids = %#v", detail.AISummary.CitationIDs)
	}
}
