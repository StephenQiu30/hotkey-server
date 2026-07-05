// Package weibo implements the Weibo hot search trending collector.
//
// Uses the public (no-auth) JSON endpoint:
//
//	GET https://weibo.com/ajax/side/hotSearch
//
// Returns the real-time hot search list with rank, title, and heat score.
package weibo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/connector"
)

const (
	defaultBaseURL = "https://weibo.com"
	hotSearchPath  = "/ajax/side/hotSearch"
	defaultTimeout = 10 * time.Second
)

// Client fetches Weibo trending data.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Weibo trending client.
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
func (c *Client) Name() string { return "weibo" }

// FetchTrending fetches the current Weibo hot search list.
func (c *Client) FetchTrending(ctx context.Context) ([]connector.TrendingItem, error) {
	url := c.baseURL + hotSearchPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("weibo: create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weibo: fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("weibo: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weibo: api returned %d: %s", resp.StatusCode, string(body))
	}

	return parseResponse(body)
}

// hotSearchResponse mirrors the Weibo hot search JSON structure.
type hotSearchResponse struct {
	Data struct {
		Realtime []struct {
			Word      string `json:"word"`
			Rank      int    `json:"rank"`
			HotNum    string `json:"hot_num"`
			Category  string `json:"category"`
			LabelName string `json:"label_name"`
			RawURL    string `json:"url"`
		} `json:"realtime"`
	} `json:"data"`
}

func parseResponse(body []byte) ([]connector.TrendingItem, error) {
	var resp hotSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("weibo: parse json: %w", err)
	}

	items := make([]connector.TrendingItem, 0, len(resp.Data.Realtime))
	now := time.Now()

	for _, entry := range resp.Data.Realtime {
		if entry.Word == "" {
			continue
		}

		heat := parseHeat(entry.HotNum)
		url := entry.RawURL
		if url != "" && !strings.HasPrefix(url, "http") {
			url = "https://weibo.com" + url
		}

		items = append(items, connector.TrendingItem{
			Platform:    "weibo",
			PlatformID:  fmt.Sprintf("weibo-%d", entry.Rank),
			Title:       entry.Word,
			Rank:        entry.Rank,
			Heat:        heat,
			URL:         url,
			Description: "",
			Category:    entry.Category,
			PublishedAt: now,
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("weibo: no trending items found in response")
	}

	return items, nil
}

// parseHeat converts Weibo's hot_num string (e.g., "1000000", "热", "沸") to float64.
func parseHeat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Special labels
	if s == "热" {
		return 500000
	}
	if s == "沸" {
		return 800000
	}
	if s == "爆" {
		return 1000000
	}
	if s == "新" {
		return 100000
	}
	if s == "荐" {
		return 50000
	}

	// Numeric: "1000000" or "1,000,000"
	s = strings.ReplaceAll(s, ",", "")
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err == nil {
		return v
	}
	return 0
}
