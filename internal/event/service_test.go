package event

import "testing"

func TestClusterSimilarItemsByVectorSimilarity(t *testing.T) {
	service := NewService(Options{VectorEnabled: true, SimilarityThreshold: 0.85})

	first, err := service.UpsertCandidate(CandidateInput{
		SourceItemID: "item_1",
		Title:        "OpenAI releases new reasoning model",
		ContentHash:  "hash-1",
		Vector:       []float64{1, 0, 0},
	})
	if err != nil {
		t.Fatalf("first UpsertCandidate returned error: %v", err)
	}
	if first.MatchMethod != MatchMethodSeed {
		t.Fatalf("first match method = %q, want %q", first.MatchMethod, MatchMethodSeed)
	}

	second, err := service.UpsertCandidate(CandidateInput{
		SourceItemID: "item_2",
		Title:        "OpenAI announces reasoning model update",
		ContentHash:  "hash-2",
		Vector:       []float64{0.95, 0.05, 0},
	})
	if err != nil {
		t.Fatalf("second UpsertCandidate returned error: %v", err)
	}
	if second.ClusterID != first.ClusterID {
		t.Fatalf("second cluster = %q, want %q", second.ClusterID, first.ClusterID)
	}
	if second.MatchMethod != MatchMethodVector {
		t.Fatalf("second match method = %q, want %q", second.MatchMethod, MatchMethodVector)
	}
	if second.Similarity < 0.85 {
		t.Fatalf("similarity = %f, want >= 0.85", second.Similarity)
	}

	clusters := service.ListClusters()
	if len(clusters) != 1 {
		t.Fatalf("clusters len = %d, want 1", len(clusters))
	}
	if len(clusters[0].Items) != 2 {
		t.Fatalf("cluster item len = %d, want 2", len(clusters[0].Items))
	}
}

func TestClusterFallsBackToRuleMatchWhenVectorUnavailable(t *testing.T) {
	service := NewService(Options{VectorEnabled: false, SimilarityThreshold: 0.85})

	first, err := service.UpsertCandidate(CandidateInput{
		SourceItemID: "item_1",
		Title:        "OpenAI releases new reasoning model",
		ContentHash:  "same-hash",
		Vector:       []float64{1, 0, 0},
	})
	if err != nil {
		t.Fatalf("first UpsertCandidate returned error: %v", err)
	}

	second, err := service.UpsertCandidate(CandidateInput{
		SourceItemID: "item_2",
		Title:        "A repost about the same model",
		ContentHash:  "same-hash",
	})
	if err != nil {
		t.Fatalf("second UpsertCandidate returned error: %v", err)
	}

	if second.ClusterID != first.ClusterID {
		t.Fatalf("second cluster = %q, want %q", second.ClusterID, first.ClusterID)
	}
	if second.MatchMethod != MatchMethodRule {
		t.Fatalf("second match method = %q, want %q", second.MatchMethod, MatchMethodRule)
	}
	if second.Similarity != 1 {
		t.Fatalf("similarity = %f, want 1", second.Similarity)
	}
}

func TestClusterRejectsInvalidCandidate(t *testing.T) {
	service := NewService(Options{VectorEnabled: true})

	_, err := service.UpsertCandidate(CandidateInput{
		SourceItemID: " ",
		Title:        "OpenAI releases new reasoning model",
		ContentHash:  "hash-1",
	})

	if err != ErrInvalidCandidate {
		t.Fatalf("err = %v, want %v", err, ErrInvalidCandidate)
	}
}
