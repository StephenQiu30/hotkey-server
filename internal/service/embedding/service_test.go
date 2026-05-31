package embedding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	"github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

func TestServiceGeneratePersistsMockEmbedding(t *testing.T) {
	repo := hotspot.NewMemoryRepository()
	items := &memoryItemRepository{items: map[string]content.SourceItem{
		"item-1": {
			ID:        "item-1",
			Title:     "OpenAI 发布新模型",
			Snippet:   "开发者正在讨论新的推理能力",
			Language:  "zh",
			CreatedAt: time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC),
		},
	}}
	service := NewService(Config{Model: "text-embedding-v2"}, items, repo, fakeProvider{
		vector: []float64{0.1, 0.2, 0.3},
		model:  "text-embedding-v2",
	})

	embedding, err := service.Generate(context.Background(), "item-1")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if embedding.ItemID != "item-1" || embedding.Model != "text-embedding-v2" || embedding.Status != hotspot.EmbeddingStatusSucceeded {
		t.Fatalf("unexpected embedding: %+v", embedding)
	}
	if got, err := repo.FindEmbedding(context.Background(), "item-1"); err != nil || len(got.Vector) != 3 {
		t.Fatalf("expected persisted vector, got embedding=%+v err=%v", got, err)
	}
}

func TestServiceGenerateMissingProviderConfigRecordsFailedConfig(t *testing.T) {
	repo := hotspot.NewMemoryRepository()
	items := &memoryItemRepository{items: map[string]content.SourceItem{
		"item-1": {ID: "item-1", Title: "AI 新闻", Snippet: "正文片段"},
	}}
	service := NewService(Config{Model: "text-embedding-v2"}, items, repo, fakeProvider{err: ErrFailedConfig})

	_, err := service.Generate(context.Background(), "item-1")
	if !errors.Is(err, ErrFailedConfig) {
		t.Fatalf("expected ErrFailedConfig, got %v", err)
	}
	embedding, findErr := repo.FindEmbedding(context.Background(), "item-1")
	if findErr != nil {
		t.Fatalf("expected failed embedding audit row, got %v", findErr)
	}
	if embedding.Status != hotspot.EmbeddingStatusFailedConfig {
		t.Fatalf("expected failed_config status, got %+v", embedding)
	}
}

func TestServiceGenerateEmptyVectorRecordsFailure(t *testing.T) {
	repo := hotspot.NewMemoryRepository()
	items := &memoryItemRepository{items: map[string]content.SourceItem{
		"item-1": {ID: "item-1", Title: "AI 新闻", Snippet: "正文片段"},
	}}
	service := NewService(Config{Model: "text-embedding-v2"}, items, repo, fakeProvider{model: "text-embedding-v2"})

	_, err := service.Generate(context.Background(), "item-1")
	if !errors.Is(err, ErrEmptyVector) {
		t.Fatalf("expected ErrEmptyVector, got %v", err)
	}
	embedding, findErr := repo.FindEmbedding(context.Background(), "item-1")
	if findErr != nil {
		t.Fatalf("expected failed embedding audit row, got %v", findErr)
	}
	if embedding.Status != hotspot.EmbeddingStatusFailed || embedding.LastError != ErrEmptyVector.Error() {
		t.Fatalf("expected failed embedding for empty vector, got %+v", embedding)
	}
}

type fakeProvider struct {
	vector []float64
	model  string
	err    error
}

func (p fakeProvider) Embed(context.Context, Request) (Response, error) {
	if p.err != nil {
		return Response{}, p.err
	}
	return Response{Vector: append([]float64(nil), p.vector...), Model: p.model}, nil
}

type memoryItemRepository struct {
	items map[string]content.SourceItem
}

func (r *memoryItemRepository) FindByID(_ context.Context, id string) (content.SourceItem, error) {
	item, ok := r.items[id]
	if !ok {
		return content.SourceItem{}, content.ErrNotFound
	}
	return item, nil
}
