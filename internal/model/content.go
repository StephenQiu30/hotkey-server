package model

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// PlatformPost is a normalized post from a platform.
type PlatformPost struct {
	ID               int64
	Platform         string
	PlatformPostID   string
	AuthorPlatformID string
	AuthorName       string
	AuthorHandle     string
	ContentText      string
	ContentLang      string
	PostURL          string
	PublishedAt      time.Time
	LikeCount        int
	ReplyCount       int
	RepostCount      int
	QuoteCount       int
	ViewCount        int
	NormalizedHash   string
}

// MonitorHit is a monitor-to-post association with scoring.
type MonitorHit struct {
	ID                  int64
	MonitorID           int64
	PostID              int64
	MatchedKeywords     pkg.JSONB[[]string]
	RelevanceScore      float64
	HeatScore           float64
	FreshnessScore      float64
	AuthorInfluenceScore float64
	FinalScore          float64
	FirstSeenAt         time.Time
	LastSeenAt          time.Time
}

// RawPost is raw data from a platform connector.
type RawPost struct {
	ID          string
	AuthorID    string
	AuthorName  string
	AuthorHandle string
	Text        string
	Language    string
	PublishedAt time.Time
	LikeCount   int
	ReplyCount  int
	RepostCount int
	QuoteCount  int
	ViewCount   int
}
