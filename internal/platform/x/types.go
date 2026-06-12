package x

import "time"

// SearchPost represents a normalized post from X search results.
type SearchPost struct {
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

// SearchMeta contains pagination metadata from X search response.
type SearchMeta struct {
	NextCursor  string
	ResultCount int
}
