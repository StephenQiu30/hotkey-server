package dedup

import (
	"context"
	"errors"
	"math"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	"github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
)

type DuplicateType string

const (
	DuplicateTypeNone  DuplicateType = "none"
	DuplicateTypeExact DuplicateType = "exact"
	DuplicateTypeNear  DuplicateType = "near"
)

type Result struct {
	DuplicateType  DuplicateType
	ExistingItemID string
	Similarity     float64
}

type Config struct {
	SimilarityThreshold float64
}

func DefaultConfig() Config {
	return Config{
		SimilarityThreshold: 0.92,
	}
}

type EmbeddingRepository interface {
	FindByItemID(ctx context.Context, itemID string) (hotspot.Embedding, error)
	List(ctx context.Context) ([]hotspot.Embedding, error)
}

type ItemRepository interface {
	FindByID(ctx context.Context, id string) (content.SourceItem, error)
	FindByContentHash(ctx context.Context, hash string) (content.SourceItem, error)
	UpdateStatus(ctx context.Context, id string, status content.ItemStatus, duplicateOf string) error
}

type Service struct {
	cfg    Config
	items  ItemRepository
	embeds EmbeddingRepository
}

func NewService(cfg Config, items ItemRepository, embeds EmbeddingRepository) *Service {
	if cfg.SimilarityThreshold <= 0 || cfg.SimilarityThreshold > 1 {
		cfg.SimilarityThreshold = 0.92
	}
	return &Service{cfg: cfg, items: items, embeds: embeds}
}

func (s *Service) CheckDuplicate(ctx context.Context, item content.SourceItem, newEmbedding []float64) (Result, error) {
	// Check exact hash duplicate first
	if existing, err := s.items.FindByContentHash(ctx, item.ContentHash); err == nil {
		return Result{
			DuplicateType:  DuplicateTypeExact,
			ExistingItemID: existing.ID,
			Similarity:     1.0,
		}, nil
	} else if !errors.Is(err, content.ErrNotFound) {
		return Result{}, err
	}

	// Check near-duplicate via embedding similarity
	if len(newEmbedding) == 0 {
		return Result{DuplicateType: DuplicateTypeNone}, nil
	}

	allEmbeds, err := s.embeds.List(ctx)
	if err != nil {
		return Result{}, err
	}

	bestSim := 0.0
	bestItemID := ""
	for _, emb := range allEmbeds {
		if emb.ItemID == item.ID || emb.Status != hotspot.EmbeddingStatusSucceeded {
			continue
		}
		sim := cosineSimilarity(newEmbedding, emb.Vector)
		if sim > bestSim {
			bestSim = sim
			bestItemID = emb.ItemID
		}
	}

	if bestSim >= s.cfg.SimilarityThreshold && bestItemID != "" {
		return Result{
			DuplicateType:  DuplicateTypeNear,
			ExistingItemID: bestItemID,
			Similarity:     bestSim,
		}, nil
	}

	return Result{DuplicateType: DuplicateTypeNone}, nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
