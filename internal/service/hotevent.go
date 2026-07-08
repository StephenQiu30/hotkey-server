package service

import (
	"context"
	"math"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

// Status constants for HotEvent lifecycle.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Trend direction constants.
const (
	TrendRising    = "rising"
	TrendStable    = "stable"
	TrendDeclining = "declining"
)

// Sentinel errors for hotevent operations.
var (
)

// PlatformWeights defines the relative weight of each platform.
var PlatformWeights = map[string]float64{
	"x":     1.0,
	"weibo": 1.0,
	"zhihu": 0.8,
	"baidu": 0.7,
	"multi": 1.0,
}

// HotEventListFilter defines filtering and pagination for List queries.
type HotEventListFilter struct {
	Status   string
	Platform string
	Sort     string // "heat_score" (default) or "last_seen"
	Limit    int
	Offset   int
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

// HotEventManager is the interface used by the HTTP handler.
type HotEventManager interface {
	ListEvents(ctx context.Context, filter HotEventListFilter) ([]*dto.HotEvent, int64, error)
	GetEventByID(ctx context.Context, id int64) (*dto.HotEvent, error)
	ListEventPosts(ctx context.Context, id int64) ([]PostBrief, error)
	GetPlatforms(ctx context.Context, eventID int64) ([]*dto.EventPlatform, error)
}

// HotEventRepository defines persistence operations for HotEvent.
type HotEventRepository interface {
	Create(ctx context.Context, event *dto.HotEvent) error
	GetByID(ctx context.Context, id int64) (*dto.HotEvent, error)
	List(ctx context.Context, filter HotEventListFilter) ([]*dto.HotEvent, int64, error)
	Update(ctx context.Context, event *dto.HotEvent) error
	ArchiveOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	AddPlatform(ctx context.Context, eventID int64, platform *dto.EventPlatform) error
	GetPlatforms(ctx context.Context, eventID int64) ([]*dto.EventPlatform, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// ComputeHeatScore calculates the composite heat score for a HotEvent.
func ComputeHeatScore(platform string, heats []float64, lastSeen time.Time) float64 {
	w := PlatformWeights[platform]
	if w == 0 {
		w = 0.5
	}

	hoursSinceUpdate := time.Since(lastSeen).Hours()
	decay := math.Exp(-0.01 * hoursSinceUpdate)

	var sum float64
	for _, h := range heats {
		sum += h * decay
	}

	return math.Round(w*sum*100) / 100
}

// DetermineTrend compares current heat to previous heat.
func DetermineTrend(current, previous float64) string {
	if current > previous*1.1 {
		return TrendRising
	}
	if current < previous*0.9 {
		return TrendDeclining
	}
	return TrendStable
}

// HotEventQueryService implements the read operations needed by the HTTP handler.
type HotEventQueryService struct {
	repo HotEventRepository
}

// NewHotEventQueryService creates a new HotEventQueryService.
func NewHotEventQueryService(repo HotEventRepository) *HotEventQueryService {
	return &HotEventQueryService{repo: repo}
}

func (s *HotEventQueryService) ListEvents(ctx context.Context, filter HotEventListFilter) ([]*dto.HotEvent, int64, error) {
	return s.repo.List(ctx, filter)
}

func (s *HotEventQueryService) GetEventByID(ctx context.Context, id int64) (*dto.HotEvent, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *HotEventQueryService) ListEventPosts(ctx context.Context, id int64) ([]PostBrief, error) {
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

func (s *HotEventQueryService) GetPlatforms(ctx context.Context, eventID int64) ([]*dto.EventPlatform, error) {
	return s.repo.GetPlatforms(ctx, eventID)
}

// Ensure HotEventQueryService implements HotEventManager interface.
var _ HotEventManager = (*HotEventQueryService)(nil)
