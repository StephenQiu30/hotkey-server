package hotevent

import (
	"context"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

// QueryService implements the read operations needed by the HTTP handler.
type QueryService struct {
	repo Repository
}

func NewQueryService(repo Repository) *QueryService {
	return &QueryService{repo: repo}
}

func (s *QueryService) ListEvents(ctx context.Context, filter ListFilter) ([]*dto.HotEvent, int64, error) {
	return s.repo.List(ctx, filter)
}

func (s *QueryService) GetEventByID(ctx context.Context, id int64) (*dto.HotEvent, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *QueryService) ListEventPosts(ctx context.Context, id int64) ([]PostBrief, error) {
	ev, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if len(ev.PostIDs) == 0 {
		return nil, nil
	}

	posts := make([]PostBrief, 0, len(ev.PostIDs))
	for _, pid := range ev.PostIDs {
		posts = append(posts, PostBrief{
			ID:     pid,
			Title:  ev.Name,
			SeenAt: ev.LastSeenAt,
		})
	}
	return posts, nil
}

func (s *QueryService) GetPlatforms(ctx context.Context, eventID int64) ([]*dto.EventPlatform, error) {
	return s.repo.GetPlatforms(ctx, eventID)
}

// PostBrief is a lightweight post for API responses.
type PostBrief struct {
	ID       int64     `json:"id"`
	Platform string    `json:"platform,omitempty"`
	Title    string    `json:"title,omitempty"`
	URL      string    `json:"url,omitempty"`
	Heat     float64   `json:"heat,omitempty"`
	SeenAt   time.Time `json:"seen_at,omitempty"`
}

// Ensure QueryService implements HotEventManager interface
var _ HotEventManager = (*QueryService)(nil)

// HotEventManager is the interface used by the HTTP handler.
type HotEventManager interface {
	ListEvents(ctx context.Context, filter ListFilter) ([]*dto.HotEvent, int64, error)
	GetEventByID(ctx context.Context, id int64) (*dto.HotEvent, error)
	ListEventPosts(ctx context.Context, id int64) ([]PostBrief, error)
	GetPlatforms(ctx context.Context, eventID int64) ([]*dto.EventPlatform, error)
}
