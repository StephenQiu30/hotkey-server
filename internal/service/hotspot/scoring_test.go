package hotspot

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	domainhotspot "github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

func TestScoreMultiSourceHigherThanSingleLowQuality(t *testing.T) {
	repo := domainhotspot.NewMemoryRepository()
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

	// Multi-source cluster: 3 items from different sources
	for i, srcID := range []string{"src-a", "src-b", "src-c"} {
		item := content.SourceItem{
			ID:         "item-multi-" + string(rune('a'+i)),
			SourceID:   srcID,
			Title:      "OpenAI 发布新模型",
			Snippet:    "模型推理能力大幅提升",
			PublishedAt: &now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := repo.SaveItem(context.Background(), item); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.SaveEmbedding(context.Background(), domainhotspot.Embedding{
			ItemID: item.ID,
			Model:  "text-embedding-v2",
			Vector: []float64{1, 0, 0},
			Status: domainhotspot.EmbeddingStatusSucceeded,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Single-source cluster: 1 item from one source
	singleItem := content.SourceItem{
		ID:         "item-single",
		SourceID:   "src-x",
		Title:      "小道消息",
		Snippet:    "未确认传闻",
		PublishedAt: &now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repo.SaveItem(context.Background(), singleItem); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveEmbedding(context.Background(), domainhotspot.Embedding{
		ItemID: singleItem.ID,
		Model:  "text-embedding-v2",
		Vector: []float64{0, 1, 0},
		Status: domainhotspot.EmbeddingStatusSucceeded,
	}); err != nil {
		t.Fatal(err)
	}

	// Create clusters
	multiCluster := domainhotspot.Cluster{
		ID:          "cluster-multi",
		Title:       "OpenAI 发布新模型",
		Keywords:    []string{"openai", "模型"},
		Centroid:    []float64{1, 0, 0},
		WindowStart: now.Add(-24 * time.Hour),
		WindowEnd:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	singleCluster := domainhotspot.Cluster{
		ID:          "cluster-single",
		Title:       "小道消息",
		Keywords:    []string{"小道", "消息"},
		Centroid:    []float64{0, 1, 0},
		WindowStart: now.Add(-24 * time.Hour),
		WindowEnd:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	multiItems := []domainhotspot.ClusterItem{
		{ClusterID: "cluster-multi", ItemID: "item-multi-a", Similarity: 1, CreatedAt: now},
		{ClusterID: "cluster-multi", ItemID: "item-multi-b", Similarity: 0.98, CreatedAt: now},
		{ClusterID: "cluster-multi", ItemID: "item-multi-c", Similarity: 0.96, CreatedAt: now},
	}
	singleItems := []domainhotspot.ClusterItem{
		{ClusterID: "cluster-single", ItemID: "item-single", Similarity: 1, CreatedAt: now},
	}
	if err := repo.ReplaceClusters(context.Background(),
		[]domainhotspot.Cluster{multiCluster, singleCluster},
		map[string][]domainhotspot.ClusterItem{
			"cluster-multi":  multiItems,
			"cluster-single": singleItems,
		},
	); err != nil {
		t.Fatal(err)
	}

	svc := NewScoringService(ScoringConfig{
		SourceCountWeight:   0.3,
		FreshnessWeight:     0.2,
		RelevanceWeight:     0.2,
		PropagationWeight:   0.2,
		QualityWeight:       0.1,
		FreshnessDecayHours: 24,
	}, repo, nil)
	svc.SetClock(func() time.Time { return now })

	scores, err := svc.ScoreClusters(context.Background(), now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}

	// Multi-source should score higher
	var multiScore, singleScore HotspotScore
	for _, s := range scores {
		switch s.ClusterID {
		case "cluster-multi":
			multiScore = s
		case "cluster-single":
			singleScore = s
		}
	}
	if multiScore.TotalScore <= singleScore.TotalScore {
		t.Fatalf("expected multi-source score %f > single-source score %f", multiScore.TotalScore, singleScore.TotalScore)
	}
	if multiScore.SourceCountScore <= singleScore.SourceCountScore {
		t.Fatalf("expected multi-source count score %f > single-source count score %f", multiScore.SourceCountScore, singleScore.SourceCountScore)
	}
}

func TestScoreExplanationContainsAllDimensions(t *testing.T) {
	repo := domainhotspot.NewMemoryRepository()
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

	item := content.SourceItem{
		ID:         "item-1",
		SourceID:   "src-1",
		Title:      "AI Agent 新突破",
		Snippet:    "自动化代理能力增强",
		PublishedAt: &now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repo.SaveItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveEmbedding(context.Background(), domainhotspot.Embedding{
		ItemID: item.ID, Model: "text-embedding-v2", Vector: []float64{1, 0, 0}, Status: domainhotspot.EmbeddingStatusSucceeded,
	}); err != nil {
		t.Fatal(err)
	}

	cluster := domainhotspot.Cluster{
		ID: "cluster-1", Title: "AI Agent 新突破", Keywords: []string{"ai", "agent"},
		Centroid: []float64{1, 0, 0}, WindowStart: now.Add(-24 * time.Hour), WindowEnd: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.ReplaceClusters(context.Background(),
		[]domainhotspot.Cluster{cluster},
		map[string][]domainhotspot.ClusterItem{
			"cluster-1": {{ClusterID: "cluster-1", ItemID: "item-1", Similarity: 1, CreatedAt: now}},
		},
	); err != nil {
		t.Fatal(err)
	}

	svc := NewScoringService(ScoringConfig{}, repo, nil)
	svc.SetClock(func() time.Time { return now })

	scores, err := svc.ScoreClusters(context.Background(), now.Add(-24*time.Hour), now)
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}
	if len(scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(scores))
	}

	score := scores[0]
	if score.Explanation == "" {
		t.Fatal("expected non-empty explanation")
	}
	if score.TotalScore <= 0 {
		t.Fatalf("expected positive total score, got %f", score.TotalScore)
	}
}

func TestScoreFreshnessDecaysOverTime(t *testing.T) {
	repo := domainhotspot.NewMemoryRepository()
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

	// Recent item
	recentTime := now.Add(-1 * time.Hour)
	recentItem := content.SourceItem{
		ID: "item-recent", SourceID: "src-1", Title: "最新消息", Snippet: "刚刚发布",
		PublishedAt: &recentTime, CreatedAt: recentTime, UpdatedAt: recentTime,
	}
	// Old item
	oldTime := now.Add(-48 * time.Hour)
	oldItem := content.SourceItem{
		ID: "item-old", SourceID: "src-2", Title: "旧消息", Snippet: "很久以前",
		PublishedAt: &oldTime, CreatedAt: oldTime, UpdatedAt: oldTime,
	}
	for _, item := range []content.SourceItem{recentItem, oldItem} {
		if err := repo.SaveItem(context.Background(), item); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.SaveEmbedding(context.Background(), domainhotspot.Embedding{
			ItemID: item.ID, Model: "text-embedding-v2", Vector: []float64{1, 0, 0}, Status: domainhotspot.EmbeddingStatusSucceeded,
		}); err != nil {
			t.Fatal(err)
		}
	}

	recentCluster := domainhotspot.Cluster{
		ID: "cluster-recent", Title: "最新消息", Keywords: []string{"最新"},
		Centroid: []float64{1, 0, 0}, WindowStart: now.Add(-24 * time.Hour), WindowEnd: now,
		CreatedAt: now, UpdatedAt: now,
	}
	oldCluster := domainhotspot.Cluster{
		ID: "cluster-old", Title: "旧消息", Keywords: []string{"旧"},
		Centroid: []float64{1, 0, 0}, WindowStart: now.Add(-72 * time.Hour), WindowEnd: now.Add(-24 * time.Hour),
		CreatedAt: oldTime, UpdatedAt: oldTime,
	}
	if err := repo.ReplaceClusters(context.Background(),
		[]domainhotspot.Cluster{recentCluster, oldCluster},
		map[string][]domainhotspot.ClusterItem{
			"cluster-recent": {{ClusterID: "cluster-recent", ItemID: "item-recent", Similarity: 1, CreatedAt: recentTime}},
			"cluster-old":    {{ClusterID: "cluster-old", ItemID: "item-old", Similarity: 1, CreatedAt: oldTime}},
		},
	); err != nil {
		t.Fatal(err)
	}

	svc := NewScoringService(ScoringConfig{FreshnessDecayHours: 24}, repo, nil)
	svc.SetClock(func() time.Time { return now })

	scores, err := svc.ScoreClusters(context.Background(), now.Add(-72*time.Hour), now)
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}

	var recentScore, oldScore HotspotScore
	for _, s := range scores {
		switch s.ClusterID {
		case "cluster-recent":
			recentScore = s
		case "cluster-old":
			oldScore = s
		}
	}
	if recentScore.FreshnessScore <= oldScore.FreshnessScore {
		t.Fatalf("expected recent freshness %f > old freshness %f", recentScore.FreshnessScore, oldScore.FreshnessScore)
	}
}
