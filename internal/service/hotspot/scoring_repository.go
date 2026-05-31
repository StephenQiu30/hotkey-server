package hotspot

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// ErrScoreNotFound is returned when a score lookup fails.
var ErrScoreNotFound = errors.New("score not found")

// MemoryScoreRepository is an in-memory implementation of ScoreRepository for testing.
type MemoryScoreRepository struct {
	mu     sync.RWMutex
	scores map[string]HotspotScore
	ids    []string
}

// NewMemoryScoreRepository creates a new in-memory score repository.
func NewMemoryScoreRepository() *MemoryScoreRepository {
	return &MemoryScoreRepository{
		scores: make(map[string]HotspotScore),
	}
}

func (r *MemoryScoreRepository) SaveScore(_ context.Context, score HotspotScore) (HotspotScore, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if score.CreatedAt.IsZero() {
		score.CreatedAt = time.Now().UTC()
	}
	if score.UpdatedAt.IsZero() {
		score.UpdatedAt = score.CreatedAt
	}
	r.scores[score.ID] = score
	if !containsID(r.ids, score.ID) {
		r.ids = append(r.ids, score.ID)
	}
	return score, nil
}

func (r *MemoryScoreRepository) ListScores(_ context.Context) ([]HotspotScore, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	scores := make([]HotspotScore, 0, len(r.ids))
	for _, id := range r.ids {
		scores = append(scores, r.scores[id])
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore > scores[j].TotalScore
	})
	return scores, nil
}

func (r *MemoryScoreRepository) ListScoresByWindow(_ context.Context, start, end time.Time) ([]HotspotScore, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var scores []HotspotScore
	for _, score := range r.scores {
		if (score.CreatedAt.Equal(start) || score.CreatedAt.After(start)) && score.CreatedAt.Before(end) {
			scores = append(scores, score)
		}
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore > scores[j].TotalScore
	})
	return scores, nil
}

func (r *MemoryScoreRepository) FindScoreByClusterID(_ context.Context, clusterID string) (HotspotScore, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, score := range r.scores {
		if score.ClusterID == clusterID {
			return score, nil
		}
	}
	return HotspotScore{}, ErrScoreNotFound
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
