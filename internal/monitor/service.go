package monitor

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"go.uber.org/zap"
)

// EmbeddingService generates text embeddings for monitor query text.
type EmbeddingService interface {
	Embed(ctx context.Context, text string) (pkg.Vector384, error)
}

// Service provides keyword monitor operations.
type Service struct {
	repo     Repository
	embedder EmbeddingService
}

// NewService creates a new monitor Service.
func NewService(repo Repository, embedder EmbeddingService) *Service {
	return &Service{repo: repo, embedder: embedder}
}

// Create validates and creates a new keyword monitor.
func (s *Service) Create(ctx context.Context, userID int64, input CreateMonitorInput) (Monitor, error) {
	if input.Name == "" || input.QueryText == "" {
		return Monitor{}, ErrInvalidInput
	}
	if _, ok := AllowedIntervals[input.PollIntervalMinutes]; !ok {
		return Monitor{}, ErrInvalidInterval
	}

	if input.Language == "" {
		input.Language = "en"
	}
	if input.Region == "" {
		input.Region = "global"
	}

	m, err := s.repo.Create(ctx, userID, input)
	if err != nil {
		return Monitor{}, err
	}

	// Generate query embedding asynchronously
	if s.embedder != nil {
		go func() {
			emb, err := s.embedder.Embed(context.Background(), input.QueryText)
			if err != nil {
				logging.L().Warn("failed to generate query embedding",
					zap.Int64("monitor_id", m.ID),
					zap.Error(err),
				)
				return
			}
			if err := s.repo.SetQueryEmbedding(context.Background(), m.ID, emb); err != nil {
				logging.L().Error("failed to save query embedding",
					zap.Int64("monitor_id", m.ID),
					zap.Error(err),
				)
			}
		}()
	}

	return m, nil
}

// GetByID retrieves a monitor by ID.
func (s *Service) GetByID(ctx context.Context, id int64) (Monitor, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Monitor{}, err
	}
	if m == nil {
		return Monitor{}, ErrNotFound
	}
	return *m, nil
}

// ListActive retrieves all active monitors regardless of user.
func (s *Service) ListActive(ctx context.Context) ([]Monitor, error) {
	return s.repo.ListActive(ctx)
}

// ListByUser retrieves all monitors for a user.
func (s *Service) ListByUser(ctx context.Context, userID int64) ([]Monitor, error) {
	return s.repo.ListByUser(ctx, userID)
}

// Update modifies an existing monitor owned by the given user.
func (s *Service) Update(ctx context.Context, id int64, userID int64, input UpdateMonitorInput) (Monitor, error) {
	if input.PollIntervalMinutes != nil {
		if _, ok := AllowedIntervals[*input.PollIntervalMinutes]; !ok {
			return Monitor{}, ErrInvalidInterval
		}
	}
	m, err := s.repo.Update(ctx, id, userID, input)
	if err != nil {
		return Monitor{}, err
	}

	// Regenerate query embedding if query_text changed
	if s.embedder != nil && input.QueryText != nil {
		go func() {
			emb, err := s.embedder.Embed(context.Background(), *input.QueryText)
			if err != nil {
				logging.L().Warn("failed to regenerate query embedding",
					zap.Int64("monitor_id", m.ID),
					zap.Error(err),
				)
				return
			}
			if err := s.repo.SetQueryEmbedding(context.Background(), m.ID, emb); err != nil {
				logging.L().Error("failed to save query embedding",
					zap.Int64("monitor_id", m.ID),
					zap.Error(err),
				)
			}
		}()
	}

	return m, nil
}
