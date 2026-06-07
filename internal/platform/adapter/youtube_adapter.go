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
	defaultYouTubeBaseURL   = "https://www.googleapis.com/youtube/v3"
	defaultMaxItemsPerFetch = 50
	defaultRateLimitPerHour = 100
	subtitleTextMaxLen      = 8000
)

// YouTubeAdapterConfig configures the YouTube adapter.
type YouTubeAdapterConfig struct {
	APIKey  string
	BaseURL string // override for testing
}

// YouTubeAdapter implements Adapter for the YouTube Data API v3.
type YouTubeAdapter struct {
	config YouTubeAdapterConfig
	client *http.Client
	health HealthInfo
	mu     sync.Mutex
}

// NewYouTubeAdapter creates a new YouTubeAdapter.
func NewYouTubeAdapter(config YouTubeAdapterConfig, client *http.Client) *YouTubeAdapter {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultYouTubeBaseURL
	}
	return &YouTubeAdapter{
		config: YouTubeAdapterConfig{
			APIKey:  config.APIKey,
			BaseURL: baseURL,
		},
		client: client,
		health: HealthInfo{Status: HealthStatusHealthy},
	}
}

func (a *YouTubeAdapter) Name() string {
	return "YouTube"
}

func (a *YouTubeAdapter) Provider() Provider {
	return ProviderYouTube
}

func (a *YouTubeAdapter) Collect(input CollectInput) (CollectOutput, error) {
	if a.config.APIKey == "" {
		return CollectOutput{}, NewAdapterError(FailureClassAuth, "YouTube API key not configured", nil)
	}

	// Parse channel ID from URL
	channelID, err := extractChannelID(input.URL)
	if err != nil {
		return CollectOutput{}, NewAdapterError(FailureClassPermanent, "invalid YouTube URL", err)
	}

	// Fetch recent videos from channel
	videos, err := a.fetchChannelVideos(channelID, input.Since)
	if err != nil {
		a.updateHealth(HealthStatusDegraded, err.Error())
		return CollectOutput{}, err
	}

	items := make([]NormalizedItem, 0, len(videos))
	for _, v := range videos {
		item := NormalizedItem{
			Title:       v.Snippet.Title,
			URL:         fmt.Sprintf("https://www.youtube.com/watch?v=%s", v.ID),
			Snippet:     v.Snippet.Description,
			ExternalID:  v.ID,
			PublishedAt: &v.Snippet.PublishedAt,
			Language:    v.Snippet.DefaultLanguage,
			Metadata: map[string]string{
				"channel_id":   v.Snippet.ChannelID,
				"channel_title": v.Snippet.ChannelTitle,
				"thumbnail":    v.Snippet.Thumbnails.Default.URL,
			},
		}

		// Try to get captions/subtitles
		captionText, err := a.fetchCaptions(v.ID)
		if err == nil && captionText != "" {
			item.Metadata["caption"] = captionText
			if len(captionText) > subtitleTextMaxLen {
				item.Snippet = captionText[:subtitleTextMaxLen]
			} else {
				item.Snippet = captionText
			}
		}

		items = append(items, item)
	}

	a.updateHealth(HealthStatusHealthy, "")
	return CollectOutput{Items: items}, nil
}

func (a *YouTubeAdapter) Health() HealthInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.health
}

func (a *YouTubeAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsIncremental: true,
		MaxItemsPerFetch:    defaultMaxItemsPerFetch,
		RateLimitPerHour:    defaultRateLimitPerHour,
	}
}

func (a *YouTubeAdapter) updateHealth(status HealthStatus, errMsg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.health.Status = status
	a.health.LastError = errMsg
	a.health.LastCheckedAt = time.Now()
}

// YouTube API response structures

type youtubeSearchResponse struct {
	Items []youtubeVideo `json:"items"`
}

type youtubeVideo struct {
	ID      string           `json:"id"`
	Snippet youtubeSnippet   `json:"snippet"`
}

type youtubeSnippet struct {
	Title           string              `json:"title"`
	Description     string              `json:"description"`
	ChannelID       string              `json:"channelId"`
	ChannelTitle    string              `json:"channelTitle"`
	PublishedAt     time.Time           `json:"publishedAt"`
	DefaultLanguage string              `json:"defaultLanguage"`
	Thumbnails      youtubeThumbnails   `json:"thumbnails"`
}

type youtubeThumbnails struct {
	Default youtubeThumbnail `json:"default"`
	Medium  youtubeThumbnail `json:"medium"`
	High    youtubeThumbnail `json:"high"`
}

type youtubeThumbnail struct {
	URL string `json:"url"`
}

type youtubeCaptionResponse struct {
	Items []youtubeCaption `json:"items"`
}

type youtubeCaption struct {
	ID         string `json:"id"`
	TrackKind  string `json:"trackKind"`
	Language   string `json:"language"`
}

type youtubeCaptionText struct {
	Text string `json:"text"`
}

func (a *YouTubeAdapter) fetchChannelVideos(channelID string, since *time.Time) ([]youtubeVideo, error) {
	url := fmt.Sprintf("%s/search?channelId=%s&type=video&part=snippet&maxResults=%d&order=date&key=%s",
		a.config.BaseURL, channelID, defaultMaxItemsPerFetch, a.config.APIKey)

	if since != nil {
		url += "&publishedAfter=" + since.Format(time.RFC3339)
	}

	resp, err := a.client.Get(url)
	if err != nil {
		return nil, NewAdapterError(FailureClassTransient, "failed to fetch YouTube videos", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, NewAdapterError(FailureClassAuth, "YouTube API authentication failed", nil)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, NewAdapterError(FailureClassRateLimit, "YouTube API rate limited", nil)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewAdapterError(FailureClassTransient, fmt.Sprintf("YouTube API returned status %d", resp.StatusCode), nil)
	}

	var result youtubeSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, NewAdapterError(FailureClassParseError, "failed to decode YouTube response", err)
	}

	return result.Items, nil
}

func (a *YouTubeAdapter) fetchCaptions(videoID string) (string, error) {
	// First, list available captions
	url := fmt.Sprintf("%s/captions?videoId=%s&part=snippet&key=%s",
		a.config.BaseURL, videoID, a.config.APIKey)

	resp, err := a.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("captions API returned status %d", resp.StatusCode)
	}

	var result youtubeCaptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Find English or first available caption
	var captionID string
	for _, c := range result.Items {
		if c.TrackKind == "standard" {
			captionID = c.ID
			if c.Language == "en" {
				break
			}
		}
	}

	if captionID == "" {
		return "", fmt.Errorf("no captions available")
	}

	// Download caption text
	captionURL := fmt.Sprintf("%s/captions/%s?key=%s&tfmt=srt",
		a.config.BaseURL, captionID, a.config.APIKey)

	capResp, err := a.client.Get(captionURL)
	if err != nil {
		return "", err
	}
	defer capResp.Body.Close()

	if capResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("caption download returned status %d", capResp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(capResp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	// Parse SRT format and extract text
	return parseSRTText(string(body)), nil
}

func extractChannelID(url string) (string, error) {
	// Handle different YouTube URL formats
	// https://www.youtube.com/channel/UCxxxxxx
	// https://www.youtube.com/@username
	// https://www.youtube.com/c/ChannelName

	if strings.Contains(url, "/channel/") {
		parts := strings.Split(url, "/channel/")
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid channel URL")
		}
		channelID := strings.Split(parts[1], "/")[0]
		if channelID == "" {
			return "", fmt.Errorf("empty channel ID")
		}
		return channelID, nil
	}

	// For @username format, we'd need to resolve it via API
	// For now, return an error suggesting to use channel ID format
	if strings.Contains(url, "/@") || strings.Contains(url, "/c/") {
		return "", fmt.Errorf("@username and /c/ URLs not supported, please use /channel/ format")
	}

	return "", fmt.Errorf("unrecognized YouTube URL format")
}

func parseSRTText(srt string) string {
	var lines []string
	for _, line := range strings.Split(srt, "\n") {
		line = strings.TrimSpace(line)
		// Skip sequence numbers and timestamps
		if line == "" || strings.Contains(line, "-->") || isNumeric(line) {
			continue
		}
		// Remove HTML tags
		line = stripHTMLTags(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, " ")
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// stripHTMLTags is defined in weibo.go
