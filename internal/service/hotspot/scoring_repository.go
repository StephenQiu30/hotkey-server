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
	r.scores[score.ID] = cloneScore(score)
	if !containsID(r.ids, score.ID) {
		r.ids = append(r.ids, score.ID)
	}
	return cloneScore(score), nil
}

func (r *MemoryScoreRepository) ListScores(_ context.Context) ([]HotspotScore, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	scores := make([]HotspotScore, 0, len(r.ids))
	for _, id := range r.ids {
		scores = append(scores, cloneScore(r.scores[id]))
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
			scores = append(scores, cloneScore(score))
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
			return cloneScore(score), nil
		}
	}
	return HotspotScore{}, ErrScoreNotFound
}

func cloneScore(score HotspotScore) HotspotScore {
	score.ChannelIDs = append([]string{}, score.ChannelIDs...)
	score.SourceRefs = append([]SourceRef{}, score.SourceRefs...)
	return score
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
