package hotspot

import "time"

// MetricsResponse is the single public metric projection used by transient
// search and persisted hotspots. Nil preserves an upstream unknown value.
type MetricsResponse struct {
	ViewCount    *int64 `json:"view_count" extensions:"x-nullable"`
	LikeCount    *int64 `json:"like_count" extensions:"x-nullable"`
	CommentCount *int64 `json:"comment_count" extensions:"x-nullable"`
	ShareCount   *int64 `json:"share_count" extensions:"x-nullable"`
}

// HotspotCardResponse is the one flat public Hotspot shape. ID is absent for
// non-persistent instant-search results; all analysis fields stay top-level.
type HotspotCardResponse struct {
	ID               *int64          `json:"id,omitempty" extensions:"x-nullable"`
	SourceType       string          `json:"source_type"`
	SourceName       string          `json:"source_name"`
	ExternalID       string          `json:"external_id"`
	ContentType      string          `json:"content_type"`
	Title            string          `json:"title"`
	Summary          string          `json:"summary"`
	CanonicalURL     string          `json:"canonical_url"`
	Author           string          `json:"author"`
	Language         string          `json:"language"`
	PublishedAt      *time.Time      `json:"published_at" extensions:"x-nullable"`
	DiscoveredAt     time.Time       `json:"discovered_at"`
	Metrics          MetricsResponse `json:"metrics"`
	HeatScore        float64         `json:"heat_score"`
	QualityState     string          `json:"quality_state" enums:"credible,suspicious,unavailable"`
	Relevance        int             `json:"relevance" minimum:"0" maximum:"100"`
	RelevanceReason  string          `json:"relevance_reason"`
	KeywordMentioned bool            `json:"keyword_mentioned"`
	Importance       string          `json:"importance" enums:"low,medium,high,urgent"`
}

func Response(card Card) HotspotCardResponse {
	return HotspotCardResponse{
		ID: card.ID, SourceType: card.SourceType, SourceName: card.SourceName,
		ExternalID: card.ExternalID, ContentType: card.ContentType, Title: card.Title,
		Summary: card.Summary, CanonicalURL: card.CanonicalURL, Author: card.Author,
		Language: card.Language, PublishedAt: card.PublishedAt, DiscoveredAt: card.DiscoveredAt,
		Metrics: MetricsResponse{
			ViewCount: card.Metrics.ViewCount, LikeCount: card.Metrics.LikeCount,
			CommentCount: card.Metrics.CommentCount, ShareCount: card.Metrics.ShareCount,
		},
		HeatScore: card.HeatScore, QualityState: card.QualityState,
		Relevance: card.Relevance, RelevanceReason: card.RelevanceReason,
		KeywordMentioned: card.KeywordMentioned, Importance: card.Importance,
	}
}
