package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
	"github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
	"github.com/StephenQiu30/hotkey-server/internal/strutil"
)

var (
	ErrFailedConfig = errors.New("failed_config: embedding provider is not configured")
	ErrEmptyVector  = errors.New("empty embedding vector")
)

type Request struct {
	Text  string
	Model string
}

type Response struct {
	Vector []float64
	Model  string
}

type Provider interface {
	Embed(context.Context, Request) (Response, error)
}

type ItemRepository interface {
	FindByID(context.Context, string) (content.SourceItem, error)
}

type Repository interface {
	SaveEmbedding(context.Context, hotspot.Embedding) (hotspot.Embedding, error)
}

type Config struct {
	Model        string
	MaxTextRunes int
	MaxRetries   int
}

type Service struct {
	cfg      Config
	items    ItemRepository
	repo     Repository
	provider Provider
	now      func() time.Time
}

func NewService(cfg Config, items ItemRepository, repo Repository, provider Provider) *Service {
	if cfg.Model == "" {
		cfg.Model = "text-embedding-v2"
	}
	if cfg.MaxTextRunes <= 0 {
		cfg.MaxTextRunes = 2048
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	return &Service{cfg: cfg, items: items, repo: repo, provider: provider, now: time.Now}
}

func (s *Service) Generate(ctx context.Context, itemID string) (hotspot.Embedding, error) {
	item, err := s.items.FindByID(ctx, itemID)
	if err != nil {
		return hotspot.Embedding{}, err
	}
	text := strutil.TrimRunes(strings.TrimSpace(item.Title+"\n"+item.Snippet), s.cfg.MaxTextRunes)
	now := s.now().UTC()

	maxAttempts := 1 + s.cfg.MaxRetries
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		response, err := s.provider.Embed(ctx, Request{Text: text, Model: s.cfg.Model})
		if err != nil {
			// Config errors are not retryable
			if errors.Is(err, ErrFailedConfig) {
				return s.saveFailure(ctx, item.ID, s.cfg.Model, text, err, now)
			}
			lastErr = err
			continue
		}
		model := response.Model
		if model == "" {
			model = s.cfg.Model
		}
		if len(response.Vector) == 0 {
			lastErr = ErrEmptyVector
			continue
		}
		return s.repo.SaveEmbedding(ctx, hotspot.Embedding{
			ItemID:    item.ID,
			Model:     model,
			Vector:    response.Vector,
			TextHash:  textHash(text),
			Status:    hotspot.EmbeddingStatusSucceeded,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return s.saveFailure(ctx, item.ID, s.cfg.Model, text, lastErr, now)
}

func (s *Service) saveFailure(ctx context.Context, itemID string, model string, text string, err error, now time.Time) (hotspot.Embedding, error) {
	status := hotspot.EmbeddingStatusFailed
	if errors.Is(err, ErrFailedConfig) {
		status = hotspot.EmbeddingStatusFailedConfig
	}
	embedding, saveErr := s.repo.SaveEmbedding(ctx, hotspot.Embedding{
		ItemID:    itemID,
		Model:     model,
		TextHash:  textHash(text),
		Status:    status,
		LastError: err.Error(),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if saveErr != nil {
		return hotspot.Embedding{}, saveErr
	}
	return embedding, err
}

func textHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
