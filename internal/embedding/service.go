package embedding

import (
	"context"
	"fmt"
	"math"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// Embedder is the ONNX model interface for generating text embeddings.
type Embedder interface {
	Embed(tokenIDs []int64) ([384]float32, error)
	Close() error
}

// Tokenizer converts text to token IDs suitable for the embedding model.
type Tokenizer interface {
	Encode(text string) []int64
}

// Service provides text-to-embedding conversion.
// It handles tokenization and L2 normalization of model output.
type Service struct {
	model     Embedder
	tokenizer Tokenizer
}

// NewService creates an embedding service with the given model and tokenizer.
func NewService(model Embedder, tokenizer Tokenizer) *Service {
	return &Service{model: model, tokenizer: tokenizer}
}

// NewServiceWithTokenizer creates a service using the default tokenizer.
func NewServiceWithTokenizer(model Embedder) *Service {
	return NewService(model, NewSimpleTokenizer())
}

// Embed converts a single text to a normalized 384-dim Vector384.
func (s *Service) Embed(ctx context.Context, text string) (pkg.Vector384, error) {
	tokenIDs := s.tokenizer.Encode(text)
	raw, err := s.model.Embed(tokenIDs)
	if err != nil {
		return pkg.Vector384{}, fmt.Errorf("embed: %w", err)
	}
	return l2Normalize(raw), nil
}

// EmbedBatch converts multiple texts to normalized vectors.
func (s *Service) EmbedBatch(ctx context.Context, texts []string) ([]pkg.Vector384, error) {
	result := make([]pkg.Vector384, len(texts))
	for i, text := range texts {
		v, err := s.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d]: %w", i, err)
		}
		result[i] = v
	}
	return result, nil
}

// Close releases the underlying model resources.
func (s *Service) Close() error {
	return s.model.Close()
}

// l2Normalize applies L2 normalization to the raw model output.
func l2Normalize(raw [384]float32) pkg.Vector384 {
	var sum float64
	for _, v := range raw {
		sum += float64(v) * float64(v)
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return pkg.Vector384{}
	}
	var result pkg.Vector384
	for i, v := range raw {
		result[i] = float32(float64(v) / norm)
	}
	return result
}
