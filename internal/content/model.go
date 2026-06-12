package content

import "time"

// RawPost represents unprocessed post data from a platform connector.
type RawPost struct {
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

// NormalizedPost represents a standardized post ready for persistence.
type NormalizedPost struct {
	Platform       string
	PlatformPostID string
	AuthorPlatformID string
	AuthorName     string
	AuthorHandle   string
	ContentText    string
	ContentLang    string
	PostURL        string
	PublishedAt    time.Time
	LikeCount      int
	ReplyCount     int
	RepostCount    int
	QuoteCount     int
	ViewCount      int
	NormalizedHash string
}

// MonitorHit represents a hit relationship between a monitor and a post.
type MonitorHit struct {
	MonitorID       int64
	PostID          int64
	MatchedKeywords []string
	RelevanceScore  float64
}
