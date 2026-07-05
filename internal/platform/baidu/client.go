// Package baidu implements the Baidu trending board collector.
//
// Baidu does not provide a public JSON API. This client parses the HTML page:
//
//	GET https://top.baidu.com/board?tab=realtime
//
// The trending data is embedded in a <script> tag with id "sanData" as JSON.
package baidu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/connector"
)

const (
	defaultBaseURL = "https://top.baidu.com"
	boardPath      = "/board?tab=realtime"
	defaultTimeout = 15 * time.Second
)

// Client fetches Baidu trending data.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Baidu trending client.
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
func (c *Client) Name() string { return "baidu" }

// FetchTrending fetches the current Baidu trending board.
func (c *Client) FetchTrending(ctx context.Context) ([]connector.TrendingItem, error) {
	url := c.baseURL + boardPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("baidu: create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("baidu: fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("baidu: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("baidu: returned %d", resp.StatusCode)
	}

	return parseHTML(body)
}

// sanDataRegex extracts the JSON blob from <script id="sanData">...</script>.
var sanDataRegex = regexp.MustCompile(`id="san-data"[^>]*>([\s\S]*?)</script>`)

// baiduCard represents a single card in the Baidu trending board.
type baiduCard struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
	URL   string `json:"url"`
	Index string `json:"index"` // ranking index
	Heat  string `json:"heatScore"`
}

// baiduSanData mirrors the sanData JSON structure.
type baiduSanData struct {
	Data struct {
		Cards []struct {
			Card []baiduCard `json:"card"`
		} `json:"cards"`
	} `json:"data"`
}

func parseHTML(body []byte) ([]connector.TrendingItem, error) {
	// Try to extract the san-data JSON script
	matches := sanDataRegex.FindSubmatch(body)
	if len(matches) < 2 {
		return nil, fmt.Errorf("baidu: san-data script not found in HTML")
	}

	var data baiduSanData
	if err := json.Unmarshal(matches[1], &data); err != nil {
		// The script content might be HTML-escaped JSON
		decoded := strings.ReplaceAll(string(matches[1]), "&quot;", "\"")
		decoded = strings.ReplaceAll(decoded, "&#39;", "'")
		decoded = strings.ReplaceAll(decoded, "&amp;", "&")
		if err := json.Unmarshal([]byte(decoded), &data); err != nil {
			return nil, fmt.Errorf("baidu: parse san-data json: %w", err)
		}
	}

	items := make([]connector.TrendingItem, 0)
	now := time.Now()

	for _, cardGroup := range data.Data.Cards {
		for _, entry := range cardGroup.Card {
			if entry.Title == "" {
				continue
			}

			rank := parseRank(entry.Index)
			heat := parseHeat(entry.Heat)

			url := entry.URL
			if url != "" && !strings.HasPrefix(url, "http") {
				url = "https://top.baidu.com" + url
			}

			items = append(items, connector.TrendingItem{
				Platform:    "baidu",
				PlatformID:  fmt.Sprintf("baidu-%d", rank),
				Title:       entry.Title,
				Rank:        rank,
				Heat:        heat,
				URL:         url,
				Description: entry.Desc,
				Category:    "",
				PublishedAt: now,
			})
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("baidu: no trending items found in response")
	}

	return items, nil
}

func parseRank(s string) int {
	s = strings.TrimSpace(s)
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}

func parseHeat(s string) float64 {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
