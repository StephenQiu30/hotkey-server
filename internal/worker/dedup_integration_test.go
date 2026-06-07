package worker

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	"github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/service/dedup"
)

func TestGenerateEmbeddingHandlerMarksNearDuplicate(t *testing.T) {
	contentRepo := content.NewMemoryRepository()
	hotspotRepo := hotspot.NewMemoryRepository()
	
	// Create an existing item with its embedding
	existingItem := content.SourceItem{
		ID:           "item-existing",
		SourceID:     "src-1",
		Title:        "Original Title",
		Snippet:      "Original content snippet",
		CanonicalURL: "https://example.com/1",
		Status:       content.ItemStatusPrimary,
	}
	contentRepo.Create(context.Background(), existingItem)
	hotspotRepo.SaveEmbedding(context.Background(), hotspot.Embedding{
		ItemID: "item-existing",
		Vector: []float64{1.0, 0.0, 0.0},
		Status: hotspot.EmbeddingStatusSucceeded,
	})

	// Create a new similar item
	newItem := content.SourceItem{
		ID:           "item-new",
		SourceID:     "src-2",
		Title:        "Similar Title",
		Snippet:      "Similar content snippet",
		CanonicalURL: "https://example.com/2",
		Status:       content.ItemStatusPrimary,
	}
	contentRepo.Create(context.Background(), newItem)

	// Setup dedup service and embedding service mock
	dedupSvc := dedup.NewService(dedup.Config{SimilarityThreshold: 0.9}, contentRepo, hotspotRepo)
	embedSvc := &mockEmbeddingService{
		result: hotspot.Embedding{
			ItemID: "item-new",
			Vector: []float64{0.99, 0.01, 0.0}, // very similar to existing
			Status: hotspot.EmbeddingStatusSucceeded,
		},
	}

	handler := NewGenerateEmbeddingHandler(embedSvc, dedupSvc, contentRepo)
	
	job := queue.Job{
		Payload: mustJSON(t, queue.GenerateEmbeddingPayload{ItemID: "item-new"}),
	}

	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	// Verify new item is marked as duplicate
	updated, err := contentRepo.FindByID(context.Background(), "item-new")
	if err != nil {
		t.Fatalf("failed to find updated item: %v", err)
	}

	if updated.Status != content.ItemStatusDuplicate {
		t.Errorf("expected status duplicate, got %s", updated.Status)
	}
	if updated.DuplicateOfItemID != "item-existing" {
		t.Errorf("expected duplicate of item-existing, got %s", updated.DuplicateOfItemID)
	}
}

type mockEmbeddingService struct {
	result hotspot.Embedding
	err    error
}

func (s *mockEmbeddingService) Generate(context.Context, string) (hotspot.Embedding, error) {
	return s.result, s.err
}
