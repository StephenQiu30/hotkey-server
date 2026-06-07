package dedup

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	"github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

func TestCheckDuplicateReturnsNoneForUniqueContent(t *testing.T) {
	items := &fakeItemRepo{}
	embeds := &fakeEmbedRepo{}
	svc := NewService(DefaultConfig(), items, embeds)

	item := content.SourceItem{ID: "item-1", ContentHash: "hash-unique"}
	embedding := []float64{0.1, 0.2, 0.3}
	result, err := svc.CheckDuplicate(context.Background(), item, embedding)
	if err != nil {
		t.Fatalf("check duplicate failed: %v", err)
	}
	if result.DuplicateType != DuplicateTypeNone {
		t.Fatalf("expected none, got %+v", result)
	}
}

func TestCheckDuplicateDetectsExactHashMatch(t *testing.T) {
	existing := content.SourceItem{ID: "item-existing", ContentHash: "hash-same"}
	items := &fakeItemRepo{
		byHash: map[string]content.SourceItem{"hash-same": existing},
	}
	embeds := &fakeEmbedRepo{}
	svc := NewService(DefaultConfig(), items, embeds)

	item := content.SourceItem{ID: "item-new", ContentHash: "hash-same"}
	result, err := svc.CheckDuplicate(context.Background(), item, []float64{0.1, 0.2})
	if err != nil {
		t.Fatalf("check duplicate failed: %v", err)
	}
	if result.DuplicateType != DuplicateTypeExact || result.ExistingItemID != "item-existing" {
		t.Fatalf("expected exact duplicate of item-existing, got %+v", result)
	}
}

func TestCheckDuplicateDetectsNearDuplicateViaEmbedding(t *testing.T) {
	existing := content.SourceItem{ID: "item-existing", ContentHash: "hash-a"}
	items := &fakeItemRepo{
		byHash: map[string]content.SourceItem{},
		byID:   map[string]content.SourceItem{"item-existing": existing},
	}
	embeds := &fakeEmbedRepo{
		vectors: map[string][]float64{
			"item-existing": {0.8, 0.6, 0.0},
		},
	}
	svc := NewService(DefaultConfig(), items, embeds)

	item := content.SourceItem{ID: "item-new", ContentHash: "hash-b"}
	newEmbedding := []float64{0.8, 0.6, 0.01} // very similar
	result, err := svc.CheckDuplicate(context.Background(), item, newEmbedding)
	if err != nil {
		t.Fatalf("check duplicate failed: %v", err)
	}
	if result.DuplicateType != DuplicateTypeNear || result.ExistingItemID != "item-existing" {
		t.Fatalf("expected near duplicate of item-existing, got %+v", result)
	}
	if result.Similarity < 0.92 {
		t.Fatalf("expected high similarity, got %f", result.Similarity)
	}
}

func TestCheckDuplicateIgnoresBelowThreshold(t *testing.T) {
	existing := content.SourceItem{ID: "item-existing", ContentHash: "hash-a"}
	items := &fakeItemRepo{
		byID: map[string]content.SourceItem{"item-existing": existing},
	}
	embeds := &fakeEmbedRepo{
		vectors: map[string][]float64{
			"item-existing": {1.0, 0.0, 0.0},
		},
	}
	svc := NewService(DefaultConfig(), items, embeds)

	item := content.SourceItem{ID: "item-new", ContentHash: "hash-b"}
	newEmbedding := []float64{0.0, 1.0, 0.0} // orthogonal
	result, err := svc.CheckDuplicate(context.Background(), item, newEmbedding)
	if err != nil {
		t.Fatalf("check duplicate failed: %v", err)
	}
	if result.DuplicateType != DuplicateTypeNone {
		t.Fatalf("expected none for low similarity, got %+v", result)
	}
}

func TestCheckDuplicateHandlesEmptyEmbedding(t *testing.T) {
	items := &fakeItemRepo{}
	embeds := &fakeEmbedRepo{}
	svc := NewService(DefaultConfig(), items, embeds)

	item := content.SourceItem{ID: "item-1", ContentHash: "hash-1"}
	result, err := svc.CheckDuplicate(context.Background(), item, nil)
	if err != nil {
		t.Fatalf("check duplicate failed: %v", err)
	}
	if result.DuplicateType != DuplicateTypeNone {
		t.Fatalf("expected none for empty embedding, got %+v", result)
	}
}

func TestCosineSimilarityIdentical(t *testing.T) {
	sim := cosineSimilarity([]float64{1, 0, 0}, []float64{1, 0, 0})
	if sim < 0.999 {
		t.Fatalf("expected ~1.0, got %f", sim)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	sim := cosineSimilarity([]float64{1, 0, 0}, []float64{0, 1, 0})
	if sim > 0.001 {
		t.Fatalf("expected ~0.0, got %f", sim)
	}
}

func TestCosineSimilarityZeroVector(t *testing.T) {
	sim := cosineSimilarity([]float64{0, 0, 0}, []float64{1, 0, 0})
	if sim != 0 {
		t.Fatalf("expected 0 for zero vector, got %f", sim)
	}
}

type fakeItemRepo struct {
	byHash map[string]content.SourceItem
	byID   map[string]content.SourceItem
}

func (r *fakeItemRepo) FindByID(_ context.Context, id string) (content.SourceItem, error) {
	if r.byID == nil {
		return content.SourceItem{}, content.ErrNotFound
	}
	item, ok := r.byID[id]
	if !ok {
		return content.SourceItem{}, content.ErrNotFound
	}
	return item, nil
}

func (r *fakeItemRepo) FindByContentHash(_ context.Context, hash string) (content.SourceItem, error) {
	if r.byHash == nil {
		return content.SourceItem{}, content.ErrNotFound
	}
	item, ok := r.byHash[hash]
	if !ok {
		return content.SourceItem{}, content.ErrNotFound
	}
	return item, nil
}

func (r *fakeItemRepo) UpdateStatus(_ context.Context, _ string, _ content.ItemStatus, _ string) error {
	return nil
}

type fakeEmbedRepo struct {
	vectors map[string][]float64
}

func (r *fakeEmbedRepo) FindEmbedding(_ context.Context, itemID string) (hotspot.Embedding, error) {
	if r.vectors == nil {
		return hotspot.Embedding{}, hotspot.ErrNotFound
	}
	vec, ok := r.vectors[itemID]
	if !ok {
		return hotspot.Embedding{}, hotspot.ErrNotFound
	}
	return hotspot.Embedding{ItemID: itemID, Vector: vec, Status: hotspot.EmbeddingStatusSucceeded}, nil
}

func (r *fakeEmbedRepo) SearchSimilar(_ context.Context, vector []float64, limit int, minSimilarity float64) ([]hotspot.SimilarityResult, error) {
	var results []hotspot.SimilarityResult
	for id, vec := range r.vectors {
		sim := cosineSimilarity(vector, vec)
		if sim >= minSimilarity {
			results = append(results, hotspot.SimilarityResult{
				ItemID:     id,
				Embedding:  hotspot.Embedding{ItemID: id, Vector: vec, Status: hotspot.EmbeddingStatusSucceeded},
				Similarity: sim,
			})
		}
	}
	return results, nil
}
