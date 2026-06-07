package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultWeiboBaseURL = "https://m.weibo.cn"
	defaultWeiboTimeout = 30 * time.Second
	weiboRateLimit      = 150 // Weibo API allows ~150 requests/hour
)

// WeiboSearchResponse represents the Weibo search API response structure.
type WeiboSearchResponse struct {
	OK   int              `json:"ok"`
	Data WeiboSearchData  `json:"data"`
	ErrNo int             `json:"errno"`
	Msg  string           `json:"msg"`
}

// WeiboSearchData contains the search result cards.
type WeiboSearchData struct {
	Cards []WeiboCard `json:"cards"`
}

// WeiboCard represents a single card in the search results.
type WeiboCard struct {
	CardType int        `json:"card_type"`
	Mblog    *WeiboMblog `json:"mblog,omitempty"`
}

// WeiboMblog represents a Weibo post.
type WeiboMblog struct {
	ID           string        `json:"id"`
	MID          string        `json:"mid"`
	Text         string        `json:"text"`
	TextRaw      string        `json:"text_raw"`
	CreatedAt    string        `json:"created_at"`
	User         WeiboUser     `json:"user"`
	RepostsCount int           `json:"reposts_count"`
	CommentsCount int          `json:"comments_count"`
	AttitudesCount int         `json:"attitudes_count"`
	Pics         []WeiboPic    `json:"pics"`
	IsLongText   bool          `json:"isLongText"`
	Visible      WeiboVisible  `json:"visible"`
	Deleted      string        `json:"deleted,omitempty"`
	PageInfo     *WeiboPageInfo `json:"page_info,omitempty"`
}

// WeiboUser represents a Weibo user.
type WeiboUser struct {
	ID         int    `json:"id"`
	ScreenName string `json:"screen_name"`
	ProfileURL string `json:"profile_url"`
}

// WeiboPic represents a Weibo picture.
type WeiboPic struct {
	URL string `json:"url"`
}

// WeiboVisible represents the visibility type of a Weibo post.
type WeiboVisible struct {
	Type int `json:"type"`
}

// WeiboPageInfo represents page info for video/article content.
type WeiboPageInfo struct {
	Type    string `json:"type"`
	PageURL string `json:"page_url"`
}

// WeiboAdapterConfig configures a WeiboAdapter.
type WeiboAdapterConfig struct {
	Name         string
	AccessToken  string
	BaseURL      string
	Timeout      time.Duration
	Keywords     []string
	ExcludeWords []string
}

// WeiboAdapter implements the Adapter interface for Weibo platform.
type WeiboAdapter struct {
	config     WeiboAdapterConfig
	baseURL    string
	client     *http.Client
	health     HealthInfo
	mu         sync.Mutex
}

// NewWeiboAdapter creates a new WeiboAdapter with the given config.
func NewWeiboAdapter(config WeiboAdapterConfig) *WeiboAdapter {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultWeiboBaseURL
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultWeiboTimeout
	}
	return &WeiboAdapter{
		config:  config,
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
		},
		health: HealthInfo{
			Status:        HealthStatusHealthy,
			LastCheckedAt: time.Now().UTC(),
		},
	}
}

func (a *WeiboAdapter) Name() string {
	return a.config.Name
}

func (a *WeiboAdapter) Provider() Provider {
	return ProviderWeibo
}

func (a *WeiboAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsIncremental: true,
		MaxItemsPerFetch:    50,
		RateLimitPerHour:    weiboRateLimit,
	}
}

func (a *WeiboAdapter) Health() HealthInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.health
}

func (a *WeiboAdapter) Collect(input CollectInput) (CollectOutput, error) {
	resp, err := a.fetchSearchResults(input.URL)
	if err != nil {
		a.updateHealth(err)
		return CollectOutput{}, err
	}

	if err := a.checkAPIError(resp); err != nil {
		a.updateHealth(err)
		return CollectOutput{}, err
	}

	items, err := a.normalizeResponse(resp, input.SourceID)
	if err != nil {
		a.updateHealth(err)
		return CollectOutput{}, err
	}

	// Apply keyword filtering
	items = a.filterByKeywords(items)

	// Ensure Items is never nil
	if items == nil {
		items = []NormalizedItem{}
	}

	a.updateHealth(nil)
	return CollectOutput{Items: items}, nil
}

func (a *WeiboAdapter) fetchSearchResults(url string) (*WeiboSearchResponse, error) {
	if url == "" {
		url = a.baseURL + "/2/search/topics"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, NewAdapterError(FailureClassPermanent, "failed to create request", err)
	}

	// Add access token as query parameter
	if a.config.AccessToken != "" {
		q := req.URL.Query()
		q.Set("access_token", a.config.AccessToken)
		req.URL.RawQuery = q.Encode()
	}

	req.Header.Set("User-Agent", "HotKey/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, NewAdapterError(FailureClassTransient, "network error", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewAdapterError(FailureClassTransient, "failed to read response body", err)
	}

	// Check HTTP status codes
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, NewAdapterError(FailureClassRateLimit, fmt.Sprintf("HTTP 429: rate limited"), nil)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, NewAdapterError(FailureClassAuth, fmt.Sprintf("HTTP %d: unauthorized", resp.StatusCode), nil)
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, NewAdapterError(FailureClassPermanent, fmt.Sprintf("HTTP %d: content not found", resp.StatusCode), nil)
	}
	if resp.StatusCode >= 400 {
		return nil, NewAdapterError(FailureClassPermanent, fmt.Sprintf("HTTP %d", resp.StatusCode), nil)
	}

	var searchResp WeiboSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, NewAdapterError(FailureClassParseError, "failed to parse JSON response", err)
	}

	return &searchResp, nil
}

func (a *WeiboAdapter) checkAPIError(resp *WeiboSearchResponse) error {
	if resp.OK != 1 {
		// Classify Weibo-specific error codes
		switch resp.ErrNo {
		case 10023, 10024:
			return NewAdapterError(FailureClassRateLimit, fmt.Sprintf("weibo rate limit: %s", resp.Msg), nil)
		case 21332, 21333:
			return NewAdapterError(FailureClassAuth, fmt.Sprintf("weibo auth error: %s", resp.Msg), nil)
		default:
			return NewAdapterError(FailureClassPermanent, fmt.Sprintf("weibo API error %d: %s", resp.ErrNo, resp.Msg), nil)
		}
	}
	return nil
}

func (a *WeiboAdapter) normalizeResponse(resp *WeiboSearchResponse, sourceID string) ([]NormalizedItem, error) {
	var items []NormalizedItem

	for _, card := range resp.Data.Cards {
		// Only process card_type=9 (status cards)
		if card.CardType != 9 || card.Mblog == nil {
			continue
		}

		mblog := card.Mblog

		// Skip deleted content
		if mblog.Deleted == "1" {
			continue
		}

		// Skip empty content (likely deleted)
		if mblog.TextRaw == "" && mblog.User.ScreenName == "" {
			continue
		}

		item := a.normalizeMblog(mblog, sourceID)
		items = append(items, item)
	}

	return items, nil
}

func (a *WeiboAdapter) normalizeMblog(mblog *WeiboMblog, sourceID string) NormalizedItem {
	// Extract title from first line or use truncated text
	title := extractTitle(mblog.TextRaw)
	snippet := stripHTMLTags(mblog.TextRaw)

	// Normalize URL to canonical form
	url := normalizeWeiboURL(mblog.ID)

	// Parse time
	publishedAt := parseWeiboTime(mblog.CreatedAt)

	// Generate idempotency key
	idempotencyKey := NewIdempotencyKey(sourceID, url)

	return NormalizedItem{
		Title:          title,
		URL:            url,
		Snippet:        snippet,
		ExternalID:     mblog.ID,
		PublishedAt:    publishedAt,
		Language:       "zh",
		IdempotencyKey: idempotencyKey,
	}
}

func (a *WeiboAdapter) filterByKeywords(items []NormalizedItem) []NormalizedItem {
	if len(a.config.Keywords) == 0 && len(a.config.ExcludeWords) == 0 {
		return items
	}

	var filtered []NormalizedItem
	for _, item := range items {
		// Check exclude words first
		if a.matchesExcludeWords(item) {
			continue
		}
		// Check keywords (if any specified)
		if len(a.config.Keywords) > 0 && !a.matchesKeywords(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func (a *WeiboAdapter) matchesKeywords(item NormalizedItem) bool {
	combined := item.Title + " " + item.Snippet
	for _, kw := range a.config.Keywords {
		if strings.Contains(combined, kw) {
			return true
		}
	}
	return false
}

func (a *WeiboAdapter) matchesExcludeWords(item NormalizedItem) bool {
	combined := item.Title + " " + item.Snippet
	for _, word := range a.config.ExcludeWords {
		if strings.Contains(combined, word) {
			return true
		}
	}
	return false
}

func (a *WeiboAdapter) updateHealth(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.health = HealthInfo{
			Status:        HealthStatusDegraded,
			LastError:     err.Error(),
			LastCheckedAt: time.Now().UTC(),
		}
	} else {
		a.health = HealthInfo{
			Status:        HealthStatusHealthy,
			LastCheckedAt: time.Now().UTC(),
		}
	}
}

// --- Helper functions ---

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(s string) string {
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, ""))
}

func extractTitle(text string) string {
	cleaned := stripHTMLTags(text)
	if cleaned == "" {
		return ""
	}
	// Use first line or first 100 chars as title
	lines := strings.SplitN(cleaned, "\n", 2)
	title := strings.TrimSpace(lines[0])
	if len(title) > 100 {
		title = title[:100] + "..."
	}
	return title
}

func normalizeWeiboURL(id string) string {
	return fmt.Sprintf("https://m.weibo.cn/detail/%s", id)
}

// parseWeiboTime parses Weibo's time format "Sat Jun 07 10:30:00 +0800 2026"
func parseWeiboTime(s string) *time.Time {
	if s == "" {
		return nil
	}

	// Try standard Weibo format
	t, err := time.Parse("Mon Jan 02 15:04:05 -0700 2006", s)
	if err != nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
