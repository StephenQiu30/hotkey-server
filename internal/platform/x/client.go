package x

import (
	"encoding/json"
	"fmt"
	"time"
)

// Client interacts with the X (Twitter) API.
type Client struct {
	token   string
	baseURL string
}

// NewClient creates a new X API client.
func NewClient(token, baseURL string) *Client {
	return &Client{token: token, baseURL: baseURL}
}

// searchResponse is the raw JSON structure from X search API.
type searchResponse struct {
	Data     []searchPostRaw `json:"data"`
	Meta     searchMetaRaw   `json:"meta"`
	Includes searchIncludes  `json:"includes"`
}

type searchPostRaw struct {
	ID            string        `json:"id"`
	AuthorID      string        `json:"author_id"`
	Text          string        `json:"text"`
	Lang          string        `json:"lang"`
	CreatedAt     string        `json:"created_at"`
	PublicMetrics publicMetrics `json:"public_metrics"`
}

type publicMetrics struct {
	LikeCount       int `json:"like_count"`
	ReplyCount      int `json:"reply_count"`
	RetweetCount    int `json:"retweet_count"`
	QuoteCount      int `json:"quote_count"`
	ImpressionCount int `json:"impression_count"`
}

type searchMetaRaw struct {
	NextToken   string `json:"next_token"`
	ResultCount int    `json:"result_count"`
}

type searchIncludes struct {
	Users []userRaw `json:"users"`
}

type userRaw struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// ParseSearchResponse parses raw JSON bytes from X search API into normalized posts and metadata.
// Author details are resolved from includes.users (joined by author_id ↔ user.id).
func (c *Client) ParseSearchResponse(data []byte) ([]SearchPost, SearchMeta, error) {
	var resp searchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, SearchMeta{}, fmt.Errorf("parse search response: %w", err)
	}

	userMap := make(map[string]userRaw, len(resp.Includes.Users))
	for _, u := range resp.Includes.Users {
		userMap[u.ID] = u
	}

	posts := make([]SearchPost, 0, len(resp.Data))
	for _, raw := range resp.Data {
		publishedAt, _ := time.Parse(time.RFC3339, raw.CreatedAt)

		authorName := ""
		authorHandle := ""
		if user, ok := userMap[raw.AuthorID]; ok {
			authorName = user.Name
			authorHandle = user.Username
		}

		posts = append(posts, SearchPost{
			ID:           raw.ID,
			AuthorID:     raw.AuthorID,
			AuthorName:   authorName,
			AuthorHandle: authorHandle,
			Text:         raw.Text,
			Language:     raw.Lang,
			PublishedAt:  publishedAt,
			LikeCount:    raw.PublicMetrics.LikeCount,
			ReplyCount:   raw.PublicMetrics.ReplyCount,
			RepostCount:  raw.PublicMetrics.RetweetCount,
			QuoteCount:   raw.PublicMetrics.QuoteCount,
			ViewCount:    raw.PublicMetrics.ImpressionCount,
		})
	}

	meta := SearchMeta{
		NextCursor:  resp.Meta.NextToken,
		ResultCount: resp.Meta.ResultCount,
	}

	return posts, meta, nil
}
