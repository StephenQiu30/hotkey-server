package adapter

import (
	"context"
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
func NewYouTubeAdapter(config YouTubeAdapterConfig) *YouTubeAdapter {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultYouTubeBaseURL
	}
	return &YouTubeAdapter{
		config: YouTubeAdapterConfig{
			APIKey:  config.APIKey,
			BaseURL: baseURL,
		},
		client: &http.Client{Timeout: 15 * time.Second},
		health: HealthInfo{
			Status:        HealthStatusHealthy,
			LastCheckedAt: time.Now().UTC(),
		},
	}
}

// Name returns the human-readable adapter name "YouTube".
func (a *YouTubeAdapter) Name() string {
	return "YouTube"
}

// Provider returns ProviderYouTube.
func (a *YouTubeAdapter) Provider() Provider {
	return ProviderYouTube
}

// Capabilities returns the adapter's supported features and limits.
func (a *YouTubeAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsIncremental: true,
		MaxItemsPerFetch:   defaultMaxItemsPerFetch,
		RateLimitPerHour:   defaultRateLimitPerHour,
	}
}

// Health returns the current adapter health status.
func (a *YouTubeAdapter) Health() HealthInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.health
}

// Collect fetches recent videos from a YouTube channel and normalizes them.
func (a *YouTubeAdapter) Collect(input CollectInput) (CollectOutput, error) {
	if input.Provider != ProviderYouTube {
		return CollectOutput{}, NewAdapterError(FailureClassPermanent, "unsupported provider", nil)
	}

	channelID := extractChannelID(input.URL)
	if channelID == "" {
		return CollectOutput{}, NewAdapterError(FailureClassPermanent, "cannot extract channel ID from URL", nil)
	}

	items, err := a.fetchChannelVideos(channelID, input)
	if err != nil {
		a.updateHealth(err)
		return CollectOutput{}, err
	}

	a.updateHealth(nil)
	return CollectOutput{Items: items}, nil
}

// fetchChannelVideos searches for recent videos in a channel and fetches their details.
func (a *YouTubeAdapter) fetchChannelVideos(channelID string, input CollectInput) ([]NormalizedItem, error) {
	searchURL := fmt.Sprintf("%s/search?key=%s&channelId=%s&part=snippet&type=video&order=date&maxResults=%d",
		a.config.BaseURL, a.config.APIKey, channelID, defaultMaxItemsPerFetch)

	if input.Since != nil {
		searchURL += "&publishedAfter=" + input.Since.UTC().Format(time.RFC3339)
	}

	body, err := a.doRequest(searchURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = body.Close()
	}()

	var resp youTubeSearchResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, NewAdapterError(FailureClassParseError, "decode search response", err)
	}

	videoIDs := make([]string, 0, len(resp.Items))
	for _, item := range resp.Items {
		videoIDs = append(videoIDs, item.ID.VideoID)
	}
	if len(videoIDs) == 0 {
		return []NormalizedItem{}, nil
	}

	videos, err := a.fetchVideoDetails(videoIDs)
	if err != nil {
		return nil, err
	}

	items := make([]NormalizedItem, 0, len(videos))
	for _, v := range videos {
		if shouldFilterVideo(v) {
			continue
		}
		item := a.normalizeVideo(v, input)
		items = append(items, item)
	}
	return items, nil
}

// fetchVideoDetails retrieves full metadata for the given video IDs.
func (a *YouTubeAdapter) fetchVideoDetails(videoIDs []string) ([]youTubeVideo, error) {
	ids := strings.Join(videoIDs, ",")
	detailURL := fmt.Sprintf("%s/videos?key=%s&id=%s&part=snippet,contentDetails,statistics,status",
		a.config.BaseURL, a.config.APIKey, ids)

	body, err := a.doRequest(detailURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = body.Close()
	}()

	var resp youTubeVideoListResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, NewAdapterError(FailureClassParseError, "decode video details", err)
	}
	return resp.Items, nil
}

// doRequest executes an HTTP GET and classifies errors by failure type.
func (a *YouTubeAdapter) doRequest(rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, NewAdapterError(FailureClassTransient, "build request", err)
	}
	req.Header.Set("User-Agent", "HotKeyBot/1.0 (+youtube-collection)")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, NewAdapterError(FailureClassTransient, "execute request", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if isYouTubeQuotaError(string(bodyBytes)) {
			return nil, NewAdapterError(FailureClassRateLimit, "YouTube API quota exceeded", nil)
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, NewAdapterError(FailureClassAuth, "YouTube API unauthorized", nil)
		}
		return nil, NewAdapterError(FailureClassRateLimit, fmt.Sprintf("YouTube API forbidden: %s", string(bodyBytes)), nil)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		_ = resp.Body.Close()
		return nil, NewAdapterError(FailureClassRateLimit, "YouTube API rate limited", nil)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, NewAdapterError(FailureClassTransient, fmt.Sprintf("YouTube API status %d: %s", resp.StatusCode, string(bodyBytes)), nil)
	}

	return resp.Body, nil
}

// normalizeVideo converts a YouTube API video into a NormalizedItem.
// Guards against nil Snippet to avoid panics on partial API responses.
func (a *YouTubeAdapter) normalizeVideo(v youTubeVideo, input CollectInput) NormalizedItem {
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", v.ID)
	idemKey := NewIdempotencyKey(input.SourceID, videoURL)

	if v.Snippet == nil {
		return NormalizedItem{
			Title:          "",
			URL:            videoURL,
			Snippet:        "",
			ExternalID:     v.ID,
			PublishedAt:    nil,
			Language:       "",
			IdempotencyKey: idemKey,
		}
	}

	snippet := v.Snippet.Description
	if len(snippet) > subtitleTextMaxLen {
		snippet = snippet[:subtitleTextMaxLen]
	}

	var publishedAt *time.Time
	if t, err := time.Parse(time.RFC3339, v.Snippet.PublishedAt); err == nil {
		publishedAt = &t
	}

	return NormalizedItem{
		Title:          strings.TrimSpace(v.Snippet.Title),
		URL:            videoURL,
		Snippet:        snippet,
		ExternalID:     v.ID,
		PublishedAt:    publishedAt,
		Language:       v.Snippet.DefaultLanguage,
		IdempotencyKey: idemKey,
	}
}

// updateHealth transitions the adapter health state based on the error.
func (a *YouTubeAdapter) updateHealth(err error) {
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

// shouldFilterVideo returns true for removed, private, or otherwise unavailable videos.
func shouldFilterVideo(v youTubeVideo) bool {
	if v.Status != nil {
		if v.Status.UploadStatus == "deleted" || v.Status.UploadStatus == "failed" {
			return true
		}
		if v.Status.PrivacyStatus == "private" {
			return true
		}
	}
	return false
}

// extractChannelID parses a channel ID from various YouTube URL formats.
func extractChannelID(rawURL string) string {
	// https://www.youtube.com/channel/UCxxxx
	if idx := strings.Index(rawURL, "/channel/"); idx >= 0 {
		rest := rawURL[idx+len("/channel/"):]
		if end := strings.IndexAny(rest, "/?#"); end >= 0 {
			return rest[:end]
		}
		return rest
	}
	// https://www.youtube.com/@handle — not supported without API lookup
	return ""
}

// isYouTubeQuotaError checks if the response body indicates a quota exceeded error.
func isYouTubeQuotaError(body string) bool {
	return strings.Contains(body, "quotaExceeded")
}

// YouTube API response types (minimal subset).

type youTubeSearchResponse struct {
	Items []youTubeSearchItem `json:"items"`
}

type youTubeSearchItem struct {
	ID struct {
		VideoID string `json:"videoId"`
	} `json:"id"`
}

type youTubeVideoListResponse struct {
	Items []youTubeVideo `json:"items"`
}

type youTubeVideo struct {
	ID      string          `json:"id"`
	Snippet *youTubeSnippet `json:"snippet"`
	Status  *youTubeStatus  `json:"status"`
}

type youTubeSnippet struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	ChannelID       string `json:"channelId"`
	ChannelTitle    string `json:"channelTitle"`
	PublishedAt     string `json:"publishedAt"`
	DefaultLanguage string `json:"defaultLanguage"`
}

type youTubeStatus struct {
	UploadStatus  string `json:"uploadStatus"`
	PrivacyStatus string `json:"privacyStatus"`
}
