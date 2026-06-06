package dedup

import (
	"context"
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	"github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
)

type DuplicateType string

const (
	DuplicateTypeNone     DuplicateType = "none"
	DuplicateTypeExact    DuplicateType = "exact"
	DuplicateTypeNear     DuplicateType = "near"
)

type Result struct {
	DuplicateType    DuplicateType
	ExistingItemID   string
	Similarity       float64
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
	cfg      Config
	items    ItemRepository
	embeds   EmbeddingRepository
}

func NewService(cfg Config, items ItemRepository, embeds EmbeddingRepository) *Service {
	if cfg.SimilarityThreshold <= 0 || cfg.SimilarityThreshold > 1 {
		cfg.SimilarityThreshold = 0.92
	}
	return &Service{cfg: cfg, items: items, embeds: embeds}
}

func (s *Service) CheckDuplicate(ctx context.Context, item content.SourceItem, newEmbedding []float64) (Result, error) {
	panic("not implemented")
}

func cosineSimilarity(a, b []float64) float64 {
	panic("not implemented")
}
