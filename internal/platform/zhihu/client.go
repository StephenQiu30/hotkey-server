// Package zhihu implements the Zhihu hot list trending collector.
//
// Uses the public API endpoint:
//
//	GET https://www.zhihu.com/api/v3/feed/topstory/hot-lists/total?limit=50
//
// No authentication required. Emits standard User-Agent header.
package zhihu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/connector"
)

const (
	defaultBaseURL = "https://www.zhihu.com"
	hotListPath    = "/api/v3/feed/topstory/hot-lists/total"
	defaultTimeout = 10 * time.Second
)

// Client fetches Zhihu trending data.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Zhihu trending client.
func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    defaultBaseURL,
	}
}

// Name returns the platform identifier.
func (c *Client) Name() string { return "zhihu" }

// FetchTrending fetches the current Zhihu hot list.
func (c *Client) FetchTrending(ctx context.Context) ([]connector.TrendingItem, error) {
	url := fmt.Sprintf("%s%s?limit=50", c.baseURL, hotListPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("zhihu: create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zhihu: fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("zhihu: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zhihu: api returned %d: %s", resp.StatusCode, string(body))
	}

	return parseResponse(body)
}

// hotListResponse mirrors the Zhihu hot list JSON structure.
type hotListResponse struct {
	Data []struct {
		Type        string `json:"type"`
		ID          string `json:"id"`
		DetailText  string `json:"detail_text"`
		MetricsArea struct {
			Text string `json:"text"`
		} `json:"metrics_area"`
		Target struct {
			ID          int64  `json:"id"`
			Title       string `json:"title"`
			URL         string `json:"url"`
			Excerpt     string `json:"excerpt"`
			AnswerCount int    `json:"answer_count"`
			FollowerCount int    `json:"follower_count"`
		} `json:"target"`
	} `json:"data"`
}

func parseResponse(body []byte) ([]connector.TrendingItem, error) {
	var resp hotListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("zhihu: parse json: %w", err)
	}

	items := make([]connector.TrendingItem, 0, len(resp.Data))
	now := time.Now()

	for i, entry := range resp.Data {
		title := entry.Target.Title
		if title == "" {
			title = entry.DetailText
		}
		if title == "" {
			continue
		}

		heat := parseMetricsHeat(entry.MetricsArea.Text)
		url := entry.Target.URL
		if url != "" && !strings.HasPrefix(url, "http") {
			url = "https://www.zhihu.com" + url
		}

		items = append(items, connector.TrendingItem{
			Platform:    "zhihu",
			PlatformID:  fmt.Sprintf("zhihu-%s", entry.ID),
			Title:       title,
			Rank:        i + 1,
			Heat:        heat,
			URL:         url,
			Description: entry.Target.Excerpt,
			Category:    "",
			PublishedAt: now,
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("zhihu: no trending items found in response")
	}

	return items, nil
}

// parseMetricsHeat extracts a numeric heat value from Zhihu's metrics text.
// Example inputs: "697 万热度", "1234 万热度", "0 热度"
func parseMetricsHeat(text string) float64 {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "热度", "")
	text = strings.TrimSpace(text)

	if strings.Contains(text, "万") {
		text = strings.ReplaceAll(text, "万", "")
		text = strings.TrimSpace(text)
		v, err := strconv.ParseFloat(text, 64)
		if err == nil {
			return v * 10000
		}
	}

	v, err := strconv.ParseFloat(text, 64)
	if err == nil {
		return v
	}
	return 0
}
