package jobs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/x"
	"github.com/StephenQiu30/hotkey-server/internal/scoring"
)

var xHTTPClient = &http.Client{Timeout: 30 * time.Second}

type XConnectorAdapter struct {
	client *x.Client
	token  string
}

func NewXConnectorAdapter(client *x.Client, token string) *XConnectorAdapter {
	return &XConnectorAdapter{client: client, token: token}
}

// SearchPosts fetches posts from the X search API.
func (a *XConnectorAdapter) SearchPosts(ctx context.Context, query string, cursor string) ([]PostResult, string, error) {
	searchURL := fmt.Sprintf("https://api.x.com/2/tweets/search/recent?query=%s", url.QueryEscape(query))
	if cursor != "" {
		searchURL += "&next_token=" + url.QueryEscape(cursor)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.token)

	resp, err := xHTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("x api returned %d: %s", resp.StatusCode, string(body))
	}

	posts, meta, err := a.client.ParseSearchResponse(body)
	if err != nil {
		return nil, "", err
	}

	results := make([]PostResult, 0, len(posts))
	for _, p := range posts {
		results = append(results, PostResult{
			ID:           p.ID,
			AuthorID:     p.AuthorID,
			AuthorName:   p.AuthorName,
			AuthorHandle: p.AuthorHandle,
			Text:         p.Text,
			Language:     p.Language,
			PublishedAt:  p.PublishedAt,
			LikeCount:    p.LikeCount,
			ReplyCount:   p.ReplyCount,
			RepostCount:  p.RepostCount,
			QuoteCount:   p.QuoteCount,
			ViewCount:    p.ViewCount,
		})
	}

	return results, meta.NextCursor, nil
}

type ScorerAdapter struct {
	svc *scoring.Service
}

func NewScorerAdapter(svc *scoring.Service) *ScorerAdapter {
	return &ScorerAdapter{svc: svc}
}

func (a *ScorerAdapter) ScoreHit(hitID int64, post PostResult, matchedKeywords []string, totalKeywords int, publishedMinutesAgo float64) error {
	return a.svc.ScoreHit(scoring.ScoreHitInput{
		HitID:               hitID,
		LikeCount:           post.LikeCount,
		ReplyCount:          post.ReplyCount,
		RepostCount:         post.RepostCount,
		QuoteCount:          post.QuoteCount,
		ViewCount:           post.ViewCount,
		MatchedKeywords:     matchedKeywords,
		TotalKeywords:       totalKeywords,
		PublishedMinutesAgo: publishedMinutesAgo,
	})
}

type MonitorLister interface {
	ListActiveIDs(ctx context.Context) ([]int64, error)
}
