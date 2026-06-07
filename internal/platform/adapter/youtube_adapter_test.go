package adapter_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
)

// searchResponseJSON builds a YouTube /search API response with the given video IDs.
// The search endpoint returns id as an object: {"videoId": "..."}.
func searchResponseJSON(videoIDs ...string) string {
	if len(videoIDs) == 0 {
		return `{"items": []}`
	}
	items := ""
	for i, vid := range videoIDs {
		if i > 0 {
			items += ","
		}
		items += fmt.Sprintf(`{"id": {"videoId": %q}, "snippet": {"title": "T", "description": "D", "channelId": "UC_test", "channelTitle": "TC", "publishedAt": "2026-01-15T10:00:00Z"}}`, vid)
	}
	return fmt.Sprintf(`{"items": [%s]}`, items)
}

// videoDetailJSON builds a YouTube /videos API response with the given fields.
func videoDetailJSON(videoID, title, desc, publishedAt, privacyStatus string) string {
	status := ""
	if privacyStatus != "" {
		status = fmt.Sprintf(`, "status": {"uploadStatus": "processed", "privacyStatus": %q}`, privacyStatus)
	}
	return fmt.Sprintf(`{"items": [{"id": %q, "snippet": {"title": %q, "description": %q, "channelId": "UC_test", "channelTitle": "TC", "publishedAt": %q}, "statistics": {"viewCount": "100", "likeCount": "10", "commentCount": "1"}, "contentDetails": {"duration": "PT5M0S"}%s}]}`,
		videoID, title, desc, publishedAt, status)
}

// videoDetailWithStatusJSON builds a /videos response with explicit upload and privacy status.
func videoDetailWithStatusJSON(videoID, uploadStatus, privacyStatus string) string {
	return fmt.Sprintf(`{"items": [{"id": %q, "snippet": {"title": "T", "description": "D", "channelId": "UC_test", "channelTitle": "TC", "publishedAt": "2026-01-15T10:00:00Z"}, "status": {"uploadStatus": %q, "privacyStatus": %q}}]}`,
		videoID, uploadStatus, privacyStatus)
}

// videoDetailWithCaptionsJSON builds a /videos response with optional caption metadata.
func videoDetailWithCaptionsJSON(videoID, title, desc string, hasCaptions bool) string {
	captions := ""
	if hasCaptions {
		captions = `, "captions": {"caption": [{"language": "en", "trackKind": "standard"}]}`
	}
	return fmt.Sprintf(`{"items": [{"id": %q, "snippet": {"title": %q, "description": %q, "channelId": "UC_test", "channelTitle": "TC", "publishedAt": "2026-01-15T10:00:00Z"}, "statistics": {"viewCount": "100", "likeCount": "10", "commentCount": "1"}, "contentDetails": {"duration": "PT5M0S"}%s}]}`,
		videoID, title, desc, captions)
}

// routeYouTubeMock creates a test server that routes /search and /videos to separate responses.
func routeYouTubeMock(searchResp, videosResp string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/search":
			_, _ = w.Write([]byte(searchResp))
		case r.URL.Path == "/videos":
			_, _ = w.Write([]byte(videosResp))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestYouTubeAdapter_Name(t *testing.T) {
	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey: "test-key",
	})
	if got := a.Name(); got != "YouTube" {
		t.Errorf("Name() = %q, want %q", got, "YouTube")
	}
}

func TestYouTubeAdapter_Provider(t *testing.T) {
	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey: "test-key",
	})
	if got := a.Provider(); got != adapter.ProviderYouTube {
		t.Errorf("Provider() = %q, want %q", got, adapter.ProviderYouTube)
	}
}

func TestYouTubeAdapter_Capabilities(t *testing.T) {
	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey: "test-key",
	})
	caps := a.Capabilities()
	if !caps.SupportsIncremental {
		t.Error("Capabilities().SupportsIncremental = false, want true")
	}
	if caps.MaxItemsPerFetch <= 0 {
		t.Errorf("Capabilities().MaxItemsPerFetch = %d, want > 0", caps.MaxItemsPerFetch)
	}
	if caps.RateLimitPerHour <= 0 {
		t.Errorf("Capabilities().RateLimitPerHour = %d, want > 0", caps.RateLimitPerHour)
	}
}

func TestYouTubeAdapter_Collect_VideoWithSubtitles(t *testing.T) {
	srv := routeYouTubeMock(
		searchResponseJSON("vid123"),
		videoDetailWithCaptionsJSON("vid123", "Test Video Title", "Test video description text", true),
	)
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	out, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test_channel",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("Collect() returned %d items, want 1", len(out.Items))
	}

	item := out.Items[0]
	if item.Title != "Test Video Title" {
		t.Errorf("Title = %q, want %q", item.Title, "Test Video Title")
	}
	if item.URL != "https://www.youtube.com/watch?v=vid123" {
		t.Errorf("URL = %q, want %q", item.URL, "https://www.youtube.com/watch?v=vid123")
	}
	if item.ExternalID != "vid123" {
		t.Errorf("ExternalID = %q, want %q", item.ExternalID, "vid123")
	}
	if item.Snippet == "" {
		t.Error("Snippet is empty, want non-empty for video with subtitles")
	}
	if item.PublishedAt == nil {
		t.Error("PublishedAt is nil, want non-nil")
	}
	if item.IdempotencyKey == "" {
		t.Error("IdempotencyKey is empty, want non-empty")
	}
}

func TestYouTubeAdapter_Collect_VideoWithoutSubtitles(t *testing.T) {
	srv := routeYouTubeMock(
		searchResponseJSON("vid456"),
		videoDetailWithCaptionsJSON("vid456", "No Subtitles Video", "This video has no subtitle track", false),
	)
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	out, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test_channel",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("Collect() returned %d items, want 1", len(out.Items))
	}

	item := out.Items[0]
	if item.Snippet != "This video has no subtitle track" {
		t.Errorf("Snippet = %q, want description text for video without subtitles", item.Snippet)
	}
}

func TestYouTubeAdapter_Collect_RemovedVideoFiltered(t *testing.T) {
	srv := routeYouTubeMock(
		searchResponseJSON("vid_removed"),
		videoDetailWithStatusJSON("vid_removed", "deleted", "private"),
	)
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	out, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test_channel",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(out.Items) != 0 {
		t.Errorf("Collect() returned %d items for removed video, want 0", len(out.Items))
	}
}

func TestYouTubeAdapter_Collect_RateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 403,
				"message": "quotaExceeded",
				"errors": [{"reason": "quotaExceeded"}]
			}
		}`))
	}))
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test_channel",
	})
	if err == nil {
		t.Fatal("Collect() error = nil, want rate limit error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Errorf("Collect() error class = %v, want FailureClassRateLimit", err)
	}
}

func TestYouTubeAdapter_Collect_TooManyRequestsRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test_channel",
	})
	if err == nil {
		t.Fatal("Collect() error = nil, want rate limit error for 429")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Errorf("Collect() error class = %v, want FailureClassRateLimit", err)
	}
}

func TestYouTubeAdapter_Collect_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 401,
				"message": "Invalid API key"
			}
		}`))
	}))
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "invalid-key",
		BaseURL: srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test_channel",
	})
	if err == nil {
		t.Fatal("Collect() error = nil, want auth error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassAuth) {
		t.Errorf("Collect() error class = %v, want FailureClassAuth", err)
	}
}

func TestYouTubeAdapter_Collect_ChannelDuplicateIdempotency(t *testing.T) {
	srv := routeYouTubeMock(
		searchResponseJSON("vid_dup"),
		videoDetailJSON("vid_dup", "Duplicate Video", "Same video from same channel", "2026-01-18T09:00:00Z", ""),
	)
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	out1, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test_channel",
	})
	if err != nil {
		t.Fatalf("first Collect() error = %v", err)
	}
	if len(out1.Items) != 1 {
		t.Fatalf("first Collect() returned %d items, want 1", len(out1.Items))
	}

	out2, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test_channel",
	})
	if err != nil {
		t.Fatalf("second Collect() error = %v", err)
	}
	if len(out2.Items) != 1 {
		t.Fatalf("second Collect() returned %d items, want 1", len(out2.Items))
	}

	if out1.Items[0].IdempotencyKey != out2.Items[0].IdempotencyKey {
		t.Errorf("IdempotencyKey mismatch: %q != %q", out1.Items[0].IdempotencyKey, out2.Items[0].IdempotencyKey)
	}
}

func TestYouTubeAdapter_Health_InitiallyHealthy(t *testing.T) {
	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey: "test-key",
	})
	h := a.Health()
	if h.Status != adapter.HealthStatusHealthy {
		t.Errorf("Health().Status = %q, want %q", h.Status, adapter.HealthStatusHealthy)
	}
}

func TestYouTubeAdapter_Health_DegradesOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	_, _ = a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test",
	})

	h := a.Health()
	if h.Status != adapter.HealthStatusDegraded {
		t.Errorf("Health().Status = %q, want %q after error", h.Status, adapter.HealthStatusDegraded)
	}
}

func TestYouTubeAdapter_Collect_MultipleVideos(t *testing.T) {
	srv := routeYouTubeMock(
		searchResponseJSON("vid_a", "vid_b"),
		`{"items": [
			{"id": "vid_a", "snippet": {"title": "Video A", "description": "Description A", "channelId": "UC_test", "channelTitle": "Test", "publishedAt": "2026-01-15T10:00:00Z"}, "statistics": {"viewCount": "100", "likeCount": "10", "commentCount": "1"}, "contentDetails": {"duration": "PT5M0S"}},
			{"id": "vid_b", "snippet": {"title": "Video B", "description": "Description B", "channelId": "UC_test", "channelTitle": "Test", "publishedAt": "2026-01-16T10:00:00Z"}, "statistics": {"viewCount": "200", "likeCount": "20", "commentCount": "2"}, "contentDetails": {"duration": "PT3M0S"}}
		]}`,
	)
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	out, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(out.Items) != 2 {
		t.Errorf("Collect() returned %d items, want 2", len(out.Items))
	}
}

func TestYouTubeAdapter_Collect_PublishedAtParsing(t *testing.T) {
	srv := routeYouTubeMock(
		searchResponseJSON("vid_time"),
		videoDetailJSON("vid_time", "Time Test", "desc", "2026-03-20T14:30:00Z", ""),
	)
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	out, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("Collect() returned %d items, want 1", len(out.Items))
	}

	expected := time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)
	if out.Items[0].PublishedAt == nil {
		t.Fatal("PublishedAt is nil")
	}
	if !out.Items[0].PublishedAt.Equal(expected) {
		t.Errorf("PublishedAt = %v, want %v", out.Items[0].PublishedAt, expected)
	}
}

func TestYouTubeAdapter_Collect_PrivateVideoFiltered(t *testing.T) {
	srv := routeYouTubeMock(
		searchResponseJSON("vid_private"),
		videoDetailWithStatusJSON("vid_private", "processed", "private"),
	)
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	out, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(out.Items) != 0 {
		t.Errorf("Collect() returned %d items for private video, want 0", len(out.Items))
	}
}

func TestYouTubeAdapter_Collect_NilSnippetNoPanic(t *testing.T) {
	// Video with nil snippet should not panic; should return minimal item.
	srv := routeYouTubeMock(
		searchResponseJSON("vid_nil_snippet"),
		`{"items": [{"id": "vid_nil_snippet", "statistics": {"viewCount": "100"}}]}`,
	)
	defer srv.Close()

	a := adapter.NewYouTubeAdapter(adapter.YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	out, err := a.Collect(adapter.CollectInput{
		SourceID: "src1",
		Provider: adapter.ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC_test",
	})
	if err != nil {
		t.Fatalf("Collect() error = %v, want nil", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("Collect() returned %d items, want 1", len(out.Items))
	}
	item := out.Items[0]
	if item.ExternalID != "vid_nil_snippet" {
		t.Errorf("ExternalID = %q, want %q", item.ExternalID, "vid_nil_snippet")
	}
	if item.URL != "https://www.youtube.com/watch?v=vid_nil_snippet" {
		t.Errorf("URL = %q, want %q", item.URL, "https://www.youtube.com/watch?v=vid_nil_snippet")
	}
	if item.IdempotencyKey == "" {
		t.Error("IdempotencyKey is empty, want non-empty even for nil snippet")
	}
}
