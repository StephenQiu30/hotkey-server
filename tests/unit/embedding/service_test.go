package embedding_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/embedding"
)

type mockModel struct{}

func (m *mockModel) Embed(tokenIDs []int64) ([384]float32, error) {
	var v [384]float32
	for i := range 384 {
		v[i] = float32(len(tokenIDs)) / 384.0
	}
	return v, nil
}

func (m *mockModel) Close() error { return nil }

func TestEmbeddingService(t *testing.T) {
	svc := embedding.NewServiceWithTokenizer(&mockModel{})
	v, err := svc.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if v.Dim() != 384 {
		t.Errorf("expected dim 384, got %d", v.Dim())
	}
	// L2-normalized output should have unit norm
	var sum float64
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("expected unit norm, got %f", sum)
	}
}

func TestEmbeddingBatch(t *testing.T) {
	svc := embedding.NewServiceWithTokenizer(&mockModel{})
	texts := []string{"a", "b", "c"}
	vecs, err := svc.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if len(vecs) != 3 {
		t.Errorf("expected 3 vectors, got %d", len(vecs))
	}
}
