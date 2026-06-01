package rss

import (
	"context"
	"sync"
	"time"
)

type MemoryFeedRepository struct {
	mu    sync.RWMutex
	feeds map[string]Feed
}

func NewMemoryFeedRepository() *MemoryFeedRepository {
	return &MemoryFeedRepository{feeds: map[string]Feed{}}
}

func (r *MemoryFeedRepository) FindByUserID(_ context.Context, userID string) (Feed, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	feed, ok := r.feeds[userID]
	if !ok {
		return Feed{}, ErrFeedNotFound
	}
	return cloneFeed(feed), nil
}

func (r *MemoryFeedRepository) FindByTokenHash(_ context.Context, tokenHash string) (Feed, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, feed := range r.feeds {
		if feed.TokenHash == tokenHash {
			return cloneFeed(feed), nil
		}
	}
	return Feed{}, ErrFeedNotFound
}

func (r *MemoryFeedRepository) Save(_ context.Context, feed Feed) (Feed, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.feeds[feed.UserID] = cloneFeed(feed)
	return cloneFeed(feed), nil
}

func (r *MemoryFeedRepository) Disable(_ context.Context, userID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	feed, ok := r.feeds[userID]
	if !ok {
		return ErrFeedNotFound
	}
	feed.Enabled = false
	feed.UpdatedAt = now
	r.feeds[userID] = feed
	return nil
}

func (r *MemoryFeedRepository) Touch(_ context.Context, userID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	feed, ok := r.feeds[userID]
	if !ok {
		return ErrFeedNotFound
	}
	feed.LastAccessedAt = &now
	feed.UpdatedAt = now
	r.feeds[userID] = feed
	return nil
}

func cloneFeed(feed Feed) Feed {
	if feed.LastAccessedAt != nil {
		t := *feed.LastAccessedAt
		feed.LastAccessedAt = &t
	}
	return feed
}
