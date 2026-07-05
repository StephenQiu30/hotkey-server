package connector

import "context"

// Searcher defines the interface for platforms that support keyword-based search.
//
// This was migrated from internal/jobs/poll_monitor.go (formerly PlatformConnector)
// to decouple the interface definition from the job scheduling framework.
type Searcher interface {
	// SearchPosts searches for posts matching the given query.
	// Returns posts, a cursor for pagination, and any error.
	SearchPosts(ctx context.Context, query string, cursor string) ([]PostResult, string, error)

	// Name returns the platform name (e.g., "x").
	Name() string
}
