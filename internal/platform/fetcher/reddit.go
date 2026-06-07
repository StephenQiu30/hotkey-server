package fetcher

import (
	"context"
	"errors"
	"net/http"
)

const defaultMaxCommentSamples = 10

// RedditConfig holds configuration for the Reddit fetcher.
type RedditConfig struct {
	AllowNSFW        bool
	MaxCommentSamples int
}

// RedditOption is a functional option for NewRedditFetcher.
type RedditOption func(*RedditConfig)

// RedditFetcher fetches posts from Reddit's public JSON API.
type RedditFetcher struct {
	client *http.Client
	config RedditConfig
}

// NewRedditFetcher creates a RedditFetcher with optional configuration.
func NewRedditFetcher(client *http.Client, opts ...RedditOption) *RedditFetcher {
	cfg := RedditConfig{
		MaxCommentSamples: defaultMaxCommentSamples,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &RedditFetcher{
		client: httpClient(client),
		config: cfg,
	}
}

// Fetch retrieves posts from a Reddit subreddit listing.
// TODO: stub — will be implemented in impl: commit.
func (f *RedditFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	if source.Type != SourceTypeReddit {
		return nil, errors.New("reddit fetcher requires reddit source")
	}
	return nil, errors.New("not implemented")
}
