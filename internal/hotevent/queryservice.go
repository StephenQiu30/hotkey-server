package hotevent

import (
	"context"
	"time"
)

// QueryService implements the read operations needed by the HTTP handler.
type QueryService struct {
	repo Repository
}

func NewQueryService(repo Repository) *QueryService {
	return &QueryService{repo: repo}
}

func (s *QueryService) ListEvents(ctx context.Context, filter ListFilter) ([]*HotEvent, int64, error) {
	return s.repo.List(ctx, filter)
}

func (s *QueryService) GetEventByID(ctx context.Context, id int64) (*HotEvent, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *QueryService) ListEventPosts(ctx context.Context, id int64) ([]PostBrief, error) {
	// Check the event exists
	ev, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Build post list from event's post_ids
	if len(ev.PostIDs) == 0 {
		return nil, nil
	}

	// In a real implementation, this would query platform_posts by IDs.
	// For now, return a brief representation.
	posts := make([]PostBrief, 0, len(ev.PostIDs))
	for _, pid := range ev.PostIDs {
		posts = append(posts, PostBrief{
			ID:        pid,
			Title:     ev.Name,
			SeenAt:    ev.LastSeenAt,
		})
	}
	return posts, nil
}

func (s *QueryService) GetPlatforms(ctx context.Context, eventID int64) ([]*EventPlatform, error) {
	return s.repo.GetPlatforms(ctx, eventID)
}

// PostBrief is a lightweight post for API responses.
type PostBrief struct {
	ID        int64     `json:"id"`
	Platform  string    `json:"platform,omitempty"`
	Title     string    `json:"title,omitempty"`
	URL       string    `json:"url,omitempty"`
	Heat      float64   `json:"heat,omitempty"`
	SeenAt    time.Time `json:"seen_at,omitempty"`
}

// Ensure QueryService implements HotEventManager interface
var _ HotEventManager = (*QueryService)(nil)

// HotEventManager is the interface used by the HTTP handler.
// Defined here to avoid import cycle.
type HotEventManager interface {
	ListEvents(ctx context.Context, filter ListFilter) ([]*HotEvent, int64, error)
	GetEventByID(ctx context.Context, id int64) (*HotEvent, error)
	ListEventPosts(ctx context.Context, id int64) ([]PostBrief, error)
	GetPlatforms(ctx context.Context, eventID int64) ([]*EventPlatform, error)
}
