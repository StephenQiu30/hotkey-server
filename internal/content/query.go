package content

// PostSummary is the query response for content flow endpoints.
type PostSummary struct {
	ID              int64    `json:"id"`
	PlatformPostID  string   `json:"platform_post_id"`
	AuthorName      string   `json:"author_name"`
	AuthorHandle    string   `json:"author_handle"`
	ContentText     string   `json:"content_text"`
	ContentLang     string   `json:"content_lang"`
	PublishedAt     string   `json:"published_at"`
	LikeCount       int      `json:"like_count"`
	ReplyCount      int      `json:"reply_count"`
	RepostCount     int      `json:"repost_count"`
	QuoteCount      int      `json:"quote_count"`
	ViewCount       int      `json:"view_count"`
	HeatScore       float64  `json:"heat_score"`
	RelevanceScore  float64  `json:"relevance_score"`
	FreshnessScore  float64  `json:"freshness_score"`
	FinalScore      float64  `json:"final_score"`
	MatchedKeywords []string `json:"matched_keywords"`
}

// PostQueryService abstracts the read side for post queries.
type PostQueryService interface {
	ListPostsByMonitor(monitorID int64, limit, offset int) ([]PostSummary, error)
}
