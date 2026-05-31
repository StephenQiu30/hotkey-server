package hotspot

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	domainhotspot "github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

func TestClusterSimilarItemsTogetherAndKeepsReferences(t *testing.T) {
	repo := domainhotspot.NewMemoryRepository()
	now := time.Date(2026, 5, 31, 2, 0, 0, 0, time.UTC)
	mustSaveCandidate(t, repo, content.SourceItem{ID: "item-1", Title: "OpenAI 发布新模型", Snippet: "模型推理能力提升", PublishedAt: &now}, []float64{1, 0, 0})
	mustSaveCandidate(t, repo, content.SourceItem{ID: "item-2", Title: "OpenAI 新模型上线", Snippet: "推理能力明显增强", PublishedAt: &now}, []float64{0.98, 0.02, 0})

	service := NewService(Config{SimilarityThreshold: 0.95, KeywordOverlapThreshold: 1, Window: 24 * time.Hour}, repo)
	result, err := service.Cluster(context.Background(), Window{Start: now.Add(-time.Hour), End: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("cluster failed: %v", err)
	}
	if len(result.Clusters) != 1 {
		t.Fatalf("expected one cluster, got %+v", result.Clusters)
	}
	items := result.ItemsByCluster[result.Clusters[0].ID]
	if len(items) != 2 || items[0].ItemID == items[1].ItemID {
		t.Fatalf("expected two referenced source items, got %+v", items)
	}
}

func TestClusterKeepsDissimilarItemsSeparate(t *testing.T) {
	repo := domainhotspot.NewMemoryRepository()
	now := time.Date(2026, 5, 31, 2, 0, 0, 0, time.UTC)
	mustSaveCandidate(t, repo, content.SourceItem{ID: "item-1", Title: "OpenAI 发布新模型", Snippet: "模型推理能力提升", PublishedAt: &now}, []float64{1, 0, 0})
	mustSaveCandidate(t, repo, content.SourceItem{ID: "item-2", Title: "新能源车销量增长", Snippet: "汽车市场环比回升", PublishedAt: &now}, []float64{0, 1, 0})

	service := NewService(Config{SimilarityThreshold: 0.95, KeywordOverlapThreshold: 1, Window: 24 * time.Hour}, repo)
	result, err := service.Cluster(context.Background(), Window{Start: now.Add(-time.Hour), End: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("cluster failed: %v", err)
	}
	if len(result.Clusters) != 2 {
		t.Fatalf("expected two separate clusters, got %+v", result.Clusters)
	}
}

func mustSaveCandidate(t *testing.T, repo *domainhotspot.MemoryRepository, item content.SourceItem, vector []float64) {
	t.Helper()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = *item.PublishedAt
	}
	item.UpdatedAt = item.CreatedAt
	if err := repo.SaveItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveEmbedding(context.Background(), domainhotspot.Embedding{
		ItemID:    item.ID,
		Model:     "text-embedding-v2",
		Vector:    vector,
		Status:    domainhotspot.EmbeddingStatusSucceeded,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}
}
