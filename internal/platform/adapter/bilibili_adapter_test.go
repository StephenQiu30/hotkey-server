package adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBiliBiliAdapter_Name(t *testing.T) {
	a := NewBiliBiliAdapter(BiliBiliAdapterConfig{}, nil)
	if a.Name() != "Bilibili" {
		t.Errorf("expected Bilibili, got %s", a.Name())
	}
}

func TestBiliBiliAdapter_Provider(t *testing.T) {
	a := NewBiliBiliAdapter(BiliBiliAdapterConfig{}, nil)
	if a.Provider() != ProviderBilibili {
		t.Errorf("expected ProviderBilibili, got %s", a.Provider())
	}
}

func TestBiliBiliAdapter_Capabilities(t *testing.T) {
	a := NewBiliBiliAdapter(BiliBiliAdapterConfig{}, nil)
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

func TestBiliBiliAdapter_Collect_InvalidURL(t *testing.T) {
	a := NewBiliBiliAdapter(BiliBiliAdapterConfig{}, nil)
	_, err := a.Collect(CollectInput{
		SourceID: "test",
		Provider: ProviderBilibili,
		URL:      "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestBiliBiliAdapter_Collect_Success(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x/space/wbi/arc/search" {
			resp := bilibiliVideoListResponse{
				Code:    0,
				Message: "0",
				Data: bilibiliVideoData{
					List: bilibiliVideoList{
						VList: []bilibiliVideo{
							{
								BVID:        "BV1xx411c7mD",
								CID:         12345,
								Title:       "Test Video",
								Description: "Test Description",
								Created:     time.Now().Unix(),
								Duration:    "120",
								Pic:         "https://example.com/pic.jpg",
								Author: bilibiliAuthor{
									MID:  12345,
									Name: "Test Author",
								},
								Stat: bilibiliStat{
									View:     1000,
									Danmaku:  50,
									Reply:    20,
									Favorite: 100,
									Coin:     30,
									Share:    10,
									Like:     500,
								},
							},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/x/player/v2" {
			resp := bilibiliSubtitleResponse{
				Code: 0,
				Data: bilibiliSubtitleData{
					Subtitle: bilibiliSubtitleInfo{
						List: []bilibiliSubtitle{
							{SubtitleURL: serverURL + "/subtitle", Language: "zh-CN"},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/subtitle" {
			resp := bilibiliSubtitleContent{
				Body: []bilibiliSubtitleSegment{
					{Content: "Hello"},
					{Content: "world"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	a := NewBiliBiliAdapter(BiliBiliAdapterConfig{
		BaseURL: server.URL,
	}, nil)

	result, err := a.Collect(CollectInput{
		SourceID: "test",
		Provider: ProviderBilibili,
		URL:      "https://space.bilibili.com/12345",
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
	if result.Items[0].Metadata["author"] != "Test Author" {
		t.Errorf("expected author 'Test Author', got '%s'", result.Items[0].Metadata["author"])
	}
}

func TestBiliBiliAdapter_Collect_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	a := NewBiliBiliAdapter(BiliBiliAdapterConfig{
		BaseURL: server.URL,
	}, nil)

	_, err := a.Collect(CollectInput{
		SourceID: "test",
		Provider: ProviderBilibili,
		URL:      "https://space.bilibili.com/12345",
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

func TestBiliBiliAdapter_Collect_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	a := NewBiliBiliAdapter(BiliBiliAdapterConfig{
		BaseURL: server.URL,
	}, nil)

	_, err := a.Collect(CollectInput{
		SourceID: "test",
		Provider: ProviderBilibili,
		URL:      "https://space.bilibili.com/12345",
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

func TestExtractBilibiliMID(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "standard space URL",
			url:  "https://space.bilibili.com/12345",
			want: "12345",
		},
		{
			name:    "invalid URL",
			url:     "https://example.com",
			wantErr: true,
		},
		{
			name:    "empty MID",
			url:     "https://space.bilibili.com/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractBilibiliMID(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractBilibiliMID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractBilibiliMID() = %v, want %v", got, tt.want)
			}
		})
	}
}
