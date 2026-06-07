package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultBilibiliBaseURL = "https://api.bilibili.com"
	// defaultMaxItemsPerFetch, defaultRateLimitPerHour, subtitleTextMaxLen are defined in youtube_adapter.go
)

// BiliBiliAdapterConfig configures the Bilibili adapter.
type BiliBiliAdapterConfig struct {
	SESSDATA   string // Cookie for authentication
	BaseURL    string // override for testing
}

// BiliBiliAdapter implements Adapter for the Bilibili API.
type BiliBiliAdapter struct {
	config BiliBiliAdapterConfig
	client *http.Client
	health HealthInfo
	mu     sync.Mutex
}

// NewBiliBiliAdapter creates a new BiliBiliAdapter.
func NewBiliBiliAdapter(config BiliBiliAdapterConfig, client *http.Client) *BiliBiliAdapter {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultBilibiliBaseURL
	}
	return &BiliBiliAdapter{
		config: BiliBiliAdapterConfig{
			SESSDATA: config.SESSDATA,
			BaseURL:  baseURL,
		},
		client: client,
		health: HealthInfo{Status: HealthStatusHealthy},
	}
}

func (a *BiliBiliAdapter) Name() string {
	return "Bilibili"
}

func (a *BiliBiliAdapter) Provider() Provider {
	return ProviderBilibili
}

func (a *BiliBiliAdapter) Collect(input CollectInput) (CollectOutput, error) {
	// Parse space ID from URL (https://space.bilibili.com/{mid})
	mid, err := extractBilibiliMID(input.URL)
	if err != nil {
		return CollectOutput{}, NewAdapterError(FailureClassPermanent, "invalid Bilibili URL", err)
	}

	// Fetch recent videos from user space
	videos, err := a.fetchUserVideos(mid, input.Since)
	if err != nil {
		a.updateHealth(HealthStatusDegraded, err.Error())
		return CollectOutput{}, err
	}

	items := make([]NormalizedItem, 0, len(videos))
	for _, v := range videos {
		item := NormalizedItem{
			Title:       v.Title,
			URL:         fmt.Sprintf("https://www.bilibili.com/video/%s", v.BVID),
			Snippet:     v.Description,
			ExternalID:  v.BVID,
			PublishedAt: timePtr(time.Unix(v.Created, 0)),
			Metadata: map[string]string{
				"mid":        fmt.Sprintf("%d", v.Author.MID),
				"author":     v.Author.Name,
				"view":       fmt.Sprintf("%d", v.Stat.View),
				"danmaku":    fmt.Sprintf("%d", v.Stat.Danmaku),
				"reply":      fmt.Sprintf("%d", v.Stat.Reply),
				"favorite":   fmt.Sprintf("%d", v.Stat.Favorite),
				"coin":       fmt.Sprintf("%d", v.Stat.Coin),
				"share":      fmt.Sprintf("%d", v.Stat.Share),
				"like":       fmt.Sprintf("%d", v.Stat.Like),
				"thumbnail":  v.Pic,
				"duration":   v.Duration,
			},
		}

		// Try to get subtitles/captions
		subtitleText, err := a.fetchSubtitles(v.CID)
		if err == nil && subtitleText != "" {
			item.Metadata["subtitle"] = subtitleText
			if len([]rune(subtitleText)) > subtitleTextMaxLen {
				item.Snippet = string([]rune(subtitleText)[:subtitleTextMaxLen])
			} else {
				item.Snippet = subtitleText
			}
		}

		items = append(items, item)
	}

	a.updateHealth(HealthStatusHealthy, "")
	return CollectOutput{Items: items}, nil
}

func (a *BiliBiliAdapter) Health() HealthInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.health
}

func (a *BiliBiliAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsIncremental: true,
		MaxItemsPerFetch:    defaultMaxItemsPerFetch,
		RateLimitPerHour:    defaultRateLimitPerHour,
	}
}

func (a *BiliBiliAdapter) updateHealth(status HealthStatus, errMsg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.health.Status = status
	a.health.LastError = errMsg
	a.health.LastCheckedAt = time.Now().UTC()
}

// Bilibili API response structures

type bilibiliVideoListResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    bilibiliVideoData `json:"data"`
}

type bilibiliVideoData struct {
	List bilibiliVideoList `json:"list"`
}

type bilibiliVideoList struct {
	VList []bilibiliVideo `json:"vlist"`
}

type bilibiliVideo struct {
	BVID        string           `json:"bvid"`
	CID         int64            `json:"cid"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Created     int64            `json:"created"`
	Duration    string           `json:"duration"`
	Pic         string           `json:"pic"`
	Author      bilibiliAuthor   `json:"author"`
	Stat        bilibiliStat     `json:"stat"`
}

type bilibiliAuthor struct {
	MID  int64  `json:"mid"`
	Name string `json:"name"`
}

type bilibiliStat struct {
	View     int64 `json:"view"`
	Danmaku  int64 `json:"danmaku"`
	Reply    int64 `json:"reply"`
	Favorite int64 `json:"favorite"`
	Coin     int64 `json:"coin"`
	Share    int64 `json:"share"`
	Like     int64 `json:"like"`
}

type bilibiliSubtitleResponse struct {
	Code int                  `json:"code"`
	Data bilibiliSubtitleData `json:"data"`
}

type bilibiliSubtitleData struct {
	Subtitle bilibiliSubtitleInfo `json:"subtitle"`
}

type bilibiliSubtitleInfo struct {
	List []bilibiliSubtitle `json:"list"`
}

type bilibiliSubtitle struct {
	SubtitleURL string `json:"subtitle_url"`
	Language    string `json:"lan"`
}

type bilibiliSubtitleContent struct {
	Body []bilibiliSubtitleSegment `json:"body"`
}

type bilibiliSubtitleSegment struct {
	Content string `json:"content"`
}

func (a *BiliBiliAdapter) fetchUserVideos(mid string, since *time.Time) ([]bilibiliVideo, error) {
	url := fmt.Sprintf("%s/x/space/wbi/arc/search?mid=%s&ps=%d&pn=1",
		a.config.BaseURL, mid, defaultMaxItemsPerFetch)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, NewAdapterError(FailureClassPermanent, "failed to create request", err)
	}

	// Add authentication cookie if available
	if a.config.SESSDATA != "" {
		req.AddCookie(&http.Cookie{Name: "SESSDATA", Value: a.config.SESSDATA})
	}

	// Add required headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, NewAdapterError(FailureClassTransient, "failed to fetch Bilibili videos", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAdapterError(FailureClassAuth, "Bilibili API authentication failed", nil)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, NewAdapterError(FailureClassRateLimit, "Bilibili API rate limited", nil)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAdapterError(FailureClassTransient, fmt.Sprintf("Bilibili API returned status %d", resp.StatusCode), nil)
	}

	var result bilibiliVideoListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, NewAdapterError(FailureClassParseError, "failed to decode Bilibili response", err)
	}

	if result.Code != 0 {
		return nil, NewAdapterError(FailureClassTransient, fmt.Sprintf("Bilibili API error: %s", result.Message), nil)
	}

	// Filter by date if since is provided
	videos := result.Data.List.VList
	if since != nil {
		filtered := make([]bilibiliVideo, 0, len(videos))
		for _, v := range videos {
			if time.Unix(v.Created, 0).After(*since) {
				filtered = append(filtered, v)
			}
		}
		videos = filtered
	}

	return videos, nil
}

func (a *BiliBiliAdapter) fetchSubtitles(cid int64) (string, error) {
	url := fmt.Sprintf("%s/x/player/v2?cid=%d", a.config.BaseURL, cid)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	if a.config.SESSDATA != "" {
		req.AddCookie(&http.Cookie{Name: "SESSDATA", Value: a.config.SESSDATA})
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("player API returned status %d", resp.StatusCode)
	}

	var result bilibiliSubtitleResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Code != 0 || len(result.Data.Subtitle.List) == 0 {
		return "", fmt.Errorf("no subtitles available")
	}

	// Get first subtitle URL
	subtitleURL := result.Data.Subtitle.List[0].SubtitleURL
	if subtitleURL == "" {
		return "", fmt.Errorf("empty subtitle URL")
	}

	// Fetch subtitle content
	subResp, err := a.client.Get(subtitleURL)
	if err != nil {
		return "", err
	}
	defer subResp.Body.Close()

	if subResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("subtitle fetch returned status %d", subResp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(subResp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var subtitleContent bilibiliSubtitleContent
	if err := json.Unmarshal(body, &subtitleContent); err != nil {
		return "", err
	}

	// Extract text from segments
	var texts []string
	for _, seg := range subtitleContent.Body {
		if seg.Content != "" {
			texts = append(texts, seg.Content)
		}
	}

	return strings.Join(texts, " "), nil
}

func extractBilibiliMID(url string) (string, error) {
	// Handle Bilibili space URL format
	// https://space.bilibili.com/{mid}
	if !strings.Contains(url, "space.bilibili.com") {
		return "", fmt.Errorf("not a Bilibili space URL")
	}

	parts := strings.Split(url, "space.bilibili.com/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid Bilibili space URL")
	}

	mid := strings.Split(parts[1], "/")[0]
	if mid == "" {
		return "", fmt.Errorf("empty MID in URL")
	}

	return mid, nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}
