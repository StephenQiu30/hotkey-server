package hotspot

import "testing"

func TestListHotspotsFiltersAndSortsByHeatTrustAndRelevance(t *testing.T) {
	service := NewEmptyService()
	service.UpsertHotspot(HotspotInput{
		ID:              "cluster_1",
		Title:           "OpenAI releases new reasoning model",
		Keywords:        []string{"OpenAI", "model"},
		Region:          "global",
		Language:        "en",
		HeatScore:       80,
		TrustScore:      90,
		SimilarityScore: 0.92,
	})
	service.UpsertHotspot(HotspotInput{
		ID:              "cluster_2",
		Title:           "Local AI product rumor",
		Keywords:        []string{"rumor"},
		Region:          "cn",
		Language:        "zh",
		HeatScore:       100,
		TrustScore:      20,
		SimilarityScore: 0.6,
	})
	service.UpsertHotspot(HotspotInput{
		ID:              "cluster_3",
		Title:           "Anthropic publishes AI safety report",
		Keywords:        []string{"Anthropic", "safety"},
		Region:          "global",
		Language:        "en",
		HeatScore:       60,
		TrustScore:      95,
		SimilarityScore: 0.85,
	})

	byHeat := service.ListHotspots(ListOptions{Region: "global", Language: "en", SortBy: SortByHeat})
	if len(byHeat) != 2 {
		t.Fatalf("byHeat len = %d, want 2", len(byHeat))
	}
	if byHeat[0].ID != "cluster_1" {
		t.Fatalf("byHeat first = %q, want cluster_1", byHeat[0].ID)
	}

	byTrust := service.ListHotspots(ListOptions{Region: "global", Language: "en", SortBy: SortByTrust})
	if byTrust[0].ID != "cluster_3" {
		t.Fatalf("byTrust first = %q, want cluster_3", byTrust[0].ID)
	}

	byKeyword := service.ListHotspots(ListOptions{Keyword: "openai", SortBy: SortByRelevance})
	if len(byKeyword) != 1 || byKeyword[0].ID != "cluster_1" {
		t.Fatalf("byKeyword = %#v, want cluster_1 only", byKeyword)
	}
}

func TestGetHotspotDetailReturnsEvidenceContentSimilarityAndRisk(t *testing.T) {
	service := NewEmptyService()
	service.UpsertHotspot(HotspotInput{
		ID:              "cluster_1",
		Title:           "OpenAI releases new reasoning model",
		Keywords:        []string{"OpenAI", "model"},
		Region:          "global",
		Language:        "en",
		HeatScore:       80,
		TrustScore:      90,
		SimilarityScore: 0.92,
		RelatedContent: []RelatedContent{
			{SourceItemID: "item_1", Title: "Research source", URL: "https://arxiv.org/abs/2605.00001"},
		},
		Evidence: EvidenceDetail{
			FactEvidenceIDs:   []string{"item_1"},
			SignalEvidenceIDs: []string{"item_2"},
			RiskLabels:        []string{"needs_followup"},
		},
	})

	detail, err := service.GetHotspotDetail("cluster_1")
	if err != nil {
		t.Fatalf("GetHotspotDetail returned error: %v", err)
	}
	if detail.ID != "cluster_1" {
		t.Fatalf("detail ID = %q, want cluster_1", detail.ID)
	}
	if len(detail.RelatedContent) != 1 {
		t.Fatalf("related content len = %d, want 1", len(detail.RelatedContent))
	}
	if detail.SimilarityScore != 0.92 {
		t.Fatalf("similarity = %f, want 0.92", detail.SimilarityScore)
	}
	if len(detail.Evidence.FactEvidenceIDs) != 1 || len(detail.Evidence.SignalEvidenceIDs) != 1 {
		t.Fatalf("evidence = %#v", detail.Evidence)
	}
	if len(detail.Evidence.RiskLabels) != 1 || detail.Evidence.RiskLabels[0] != "needs_followup" {
		t.Fatalf("risk labels = %#v", detail.Evidence.RiskLabels)
	}
}

func TestGetHotspotDetailReturnsNotFound(t *testing.T) {
	service := NewEmptyService()

	_, err := service.GetHotspotDetail("missing")
	if err != ErrHotspotNotFound {
		t.Fatalf("err = %v, want %v", err, ErrHotspotNotFound)
	}
}
