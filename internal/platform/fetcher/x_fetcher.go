package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimitError is returned when the X API responds with HTTP 429.
type RateLimitError struct {
	ResetAt time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("x api rate limit exceeded; resets at %s", e.ResetAt.Format(time.RFC3339))
}

// AuthError is returned when the X API responds with HTTP 401 or 403.
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("x api auth error: %s", e.Message)
}

// XFetcher fetches posts from X API v2 recent search endpoint.
type XFetcher struct {
	client      *http.Client
	accessToken string
}

// NewXFetcher creates a new XFetcher with the given HTTP client and access token.
func NewXFetcher(client *http.Client, accessToken string) *XFetcher {
	return &XFetcher{
		client:      httpClient(client),
		accessToken: accessToken,
	}
}

// Fetch retrieves posts from X API v2 recent search.
func (f *XFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	if source.Type != SourceTypeX {
		return nil, errors.New("x fetcher requires x source type")
	}
	if strings.TrimSpace(f.accessToken) == "" {
		return nil, errors.New("x fetcher requires access token")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create x request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	req.Header.Set("User-Agent", "HotKeyBot/1.0 (+x-platform-collection)")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("x api request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := handleXAPIError(resp); err != nil {
		return nil, err
	}

	var apiResp xAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode x response: %w", err)
	}

	items := make([]Item, 0, len(apiResp.Data))
	for _, tweet := range apiResp.Data {
		item := Item{
			Title:      strings.TrimSpace(tweet.Text),
			URL:        buildXTweetURL(tweet.ID),
			ExternalID: strings.TrimSpace(tweet.ID),
		}
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(tweet.CreatedAt)); err == nil {
			item.PublishedAt = &parsed
		}
		if item.Title == "" && item.URL == "" {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func handleXAPIError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusTooManyRequests:
		resetAt := time.Now().Add(15 * time.Minute)
		if resetHeader := resp.Header.Get("x-rate-limit-reset"); resetHeader != "" {
			if resetUnix, err := strconv.ParseInt(resetHeader, 10, 64); err == nil {
				resetAt = time.Unix(resetUnix, 0)
			}
		}
		return &RateLimitError{ResetAt: resetAt}
	case http.StatusUnauthorized, http.StatusForbidden:
		return &AuthError{Message: "invalid or expired access token"}
	default:
		return fmt.Errorf("x api error: status %d", resp.StatusCode)
	}
}

func buildXTweetURL(tweetID string) string {
	if tweetID == "" {
		return ""
	}
	return fmt.Sprintf("https://x.com/i/status/%s", tweetID)
}

type xAPIResponse struct {
	Data []xTweet `json:"data"`
	Meta xMeta    `json:"meta"`
}

type xTweet struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
	AuthorID  string `json:"author_id"`
}

type xMeta struct {
	ResultCount int `json:"result_count"`
}
