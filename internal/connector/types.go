// Package connector defines the abstraction layer for external data source integration.
//
// It decouples platform-specific client implementations from the job scheduling
// framework, allowing new data sources to be added without modifying the job runner.
package connector

import "time"

// PostResult is a normalized post/status from any platform.
// Used by the search-oriented pipeline (currently X).
type PostResult struct {
	ID           string
	AuthorID     string
	AuthorName   string
	AuthorHandle string
	Text         string
	Language     string
	PublishedAt  time.Time
	LikeCount    int
	ReplyCount   int
	RepostCount  int
	QuoteCount   int
	ViewCount    int
}

// SearchResult is a richer result from the Searcher interface.
// It extends PostResult with platform metadata for multi-platform use.
type SearchResult struct {
	Platform        string
	PlatformPostID  string
	Title           string
	Content         string
	AuthorName      string
	AuthorHandle    string
	URL             string
	PublishedAt     time.Time
	LikeCount       int
	ReplyCount      int
	Extra           map[string]any
}

// TrendingItem represents a single entry from a platform's trending/hot list.
type TrendingItem struct {
	Platform    string
	PlatformID  string
	Title       string
	Rank        int
	Heat        float64
	URL         string
	Description string
	Category    string
	PublishedAt time.Time
}
