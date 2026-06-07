package adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestYouTubeAdapter_Name(t *testing.T) {
	a := NewYouTubeAdapter(YouTubeAdapterConfig{APIKey: "test"}, nil)
	if a.Name() != "YouTube" {
		t.Errorf("expected YouTube, got %s", a.Name())
	}
}

func TestYouTubeAdapter_Provider(t *testing.T) {
	a := NewYouTubeAdapter(YouTubeAdapterConfig{APIKey: "test"}, nil)
	if a.Provider() != ProviderYouTube {
		t.Errorf("expected ProviderYouTube, got %s", a.Provider())
	}
}

func TestYouTubeAdapter_Capabilities(t *testing.T) {
	a := NewYouTubeAdapter(YouTubeAdapterConfig{APIKey: "test"}, nil)
	caps := a.Capabilities()
	if !caps.SupportsIncremental {
		t.Error("expected SupportsIncremental to be true")
	}
	if caps.MaxItemsPerFetch != 50 {
		t.Errorf("expected MaxItemsPerFetch 50, got %d", caps.MaxItemsPerFetch)
	}
	if caps.RateLimitPerHour != 100 {
		t.Errorf("expected RateLimitPerHour 100, got %d", caps.RateLimitPerHour)
	}
}

func TestYouTubeAdapter_Collect_NoAPIKey(t *testing.T) {
	a := NewYouTubeAdapter(YouTubeAdapterConfig{}, nil)
	_, err := a.Collect(CollectInput{
		SourceID: "test",
		Provider: ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC123",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	adapterErr, ok := err.(*AdapterError)
	if !ok {
		t.Fatalf("expected AdapterError, got %T", err)
	}
	if adapterErr.Class != FailureClassAuth {
		t.Errorf("expected FailureClassAuth, got %s", adapterErr.Class)
	}
}

func TestYouTubeAdapter_Collect_InvalidURL(t *testing.T) {
	a := NewYouTubeAdapter(YouTubeAdapterConfig{APIKey: "test"}, nil)
	_, err := a.Collect(CollectInput{
		SourceID: "test",
		Provider: ProviderYouTube,
		URL:      "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestYouTubeAdapter_Collect_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			resp := youtubeSearchResponse{
				Items: []youtubeVideo{
					{
						ID: "video1",
						Snippet: youtubeSnippet{
							Title:        "Test Video",
							Description:  "Test Description",
							ChannelID:    "UC123",
							ChannelTitle: "Test Channel",
							PublishedAt:  time.Now(),
							Thumbnails: youtubeThumbnails{
								Default: youtubeThumbnail{URL: "https://example.com/thumb.jpg"},
							},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/captions" {
			resp := youtubeCaptionResponse{
				Items: []youtubeCaption{
					{ID: "caption1", TrackKind: "standard", Language: "en"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/captions/caption1" {
			w.Write([]byte("1\n00:00:00,000 --> 00:00:01,000\nHello world\n\n"))
		}
	}))
	defer server.Close()

	a := NewYouTubeAdapter(YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, nil)

	result, err := a.Collect(CollectInput{
		SourceID: "test",
		Provider: ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Title != "Test Video" {
		t.Errorf("expected title 'Test Video', got '%s'", result.Items[0].Title)
	}
}

func TestYouTubeAdapter_Collect_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	a := NewYouTubeAdapter(YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, nil)

	_, err := a.Collect(CollectInput{
		SourceID: "test",
		Provider: ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC123",
	})
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
	adapterErr, ok := err.(*AdapterError)
	if !ok {
		t.Fatalf("expected AdapterError, got %T", err)
	}
	if adapterErr.Class != FailureClassRateLimit {
		t.Errorf("expected FailureClassRateLimit, got %s", adapterErr.Class)
	}
}

func TestYouTubeAdapter_Collect_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	a := NewYouTubeAdapter(YouTubeAdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	}, nil)

	_, err := a.Collect(CollectInput{
		SourceID: "test",
		Provider: ProviderYouTube,
		URL:      "https://www.youtube.com/channel/UC123",
	})
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
	adapterErr, ok := err.(*AdapterError)
	if !ok {
		t.Fatalf("expected AdapterError, got %T", err)
	}
	if adapterErr.Class != FailureClassAuth {
		t.Errorf("expected FailureClassAuth, got %s", adapterErr.Class)
	}
}

func TestExtractChannelID(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "standard channel URL",
			url:  "https://www.youtube.com/channel/UC123456789",
			want: "UC123456789",
		},
		{
			name:    "invalid URL",
			url:     "https://example.com",
			wantErr: true,
		},
		{
			name:    "username URL not supported",
			url:     "https://www.youtube.com/@username",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractChannelID(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractChannelID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractChannelID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSRTText(t *testing.T) {
	srt := `1
00:00:00,000 --> 00:00:01,000
Hello world

2
00:00:01,000 --> 00:00:02,000
<b>This is bold</b>
`
	expected := "Hello world This is bold"
	got := parseSRTText(srt)
	if got != expected {
		t.Errorf("parseSRTText() = %q, want %q", got, expected)
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<b>bold</b>", "bold"},
		{"<i>italic</i>", "italic"},
		{"no tags", "no tags"},
		{"<br/>line", "line"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripHTMLTags(tt.input)
			if got != tt.want {
				t.Errorf("stripHTMLTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
