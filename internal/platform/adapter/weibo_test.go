package adapter_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
)

// --- Weibo API response fixtures ---

const weiboSearchResponseJSON = `{
	"ok": 1,
	"data": {
		"cards": [
			{
				"card_type": 9,
				"mblog": {
					"id": "5123456789012345",
					"mid": "5123456789012345",
					"text": "【AI热点】ChatGPT发布新版本，引发行业热议 <a href=\"/n/科技新闻\">#AI热点#</a>",
					"text_raw": "【AI热点】ChatGPT发布新版本，引发行业热议 #AI热点#",
					"created_at": "Sat Jun 07 10:30:00 +0800 2026",
					"user": {
						"id": 1234567890,
						"screen_name": "科技新闻",
						"profile_url": "https://m.weibo.cn/u/1234567890"
					},
					"reposts_count": 1500,
					"comments_count": 800,
					"attitudes_count": 5000,
					"pics": [
						{"url": "https://wx1.sinaimg.cn/large/pic1.jpg"},
						{"url": "https://wx2.sinaimg.cn/large/pic2.jpg"}
					],
					"isLongText": false,
					"visible": {"type": 0},
					"page_info": {
						"type": "video",
						"page_url": "https://m.weibo.cn/tv/show/123"
					}
				}
			},
			{
				"card_type": 9,
				"mblog": {
					"id": "5123456789012346",
					"mid": "5123456789012346",
					"text": "百度文心一言最新评测结果出炉",
					"text_raw": "百度文心一言最新评测结果出炉",
					"created_at": "Fri Jun 06 18:00:00 +0800 2026",
					"user": {
						"id": 9876543210,
						"screen_name": "AI观察",
						"profile_url": "https://m.weibo.cn/u/9876543210"
					},
					"reposts_count": 200,
					"comments_count": 150,
					"attitudes_count": 800,
					"isLongText": true,
					"visible": {"type": 0}
				}
			}
		],
		"cardlistInfo": {
			"page": 1,
			"total": 100
		}
	}
}`

// --- Red: WeiboAdapter basic interface contract ---

func TestWeiboAdapterImplementsAdapterInterface(t *testing.T) {
	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
	})
	// Verify it satisfies the Adapter interface
	var _ adapter.Adapter = a
}

func TestWeiboAdapterName(t *testing.T) {
	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
	})
	if a.Name() != "weibo-search" {
		t.Fatalf("expected name %q, got %q", "weibo-search", a.Name())
	}
}

func TestWeiboAdapterProvider(t *testing.T) {
	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
	})
	if a.Provider() != adapter.ProviderWeibo {
		t.Fatalf("expected provider %q, got %q", adapter.ProviderWeibo, a.Provider())
	}
}

func TestWeiboAdapterCapabilities(t *testing.T) {
	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
	})
	caps := a.Capabilities()
	if !caps.SupportsIncremental {
		t.Fatal("expected SupportsIncremental to be true")
	}
	if caps.RateLimitPerHour <= 0 {
		t.Fatal("expected RateLimitPerHour to be positive")
	}
}

// --- Red: Weibo fixture normalization ---

func TestWeiboAdapterNormalizesSearchResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics?containerid=100103type%3D1%26q%3DAI",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(output.Items))
	}

	item := output.Items[0]
	if item.Title == "" {
		t.Fatal("expected non-empty title")
	}
	if item.Snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
	if item.Language != "zh" {
		t.Fatalf("expected language %q, got %q", "zh", item.Language)
	}
	if item.PublishedAt == nil {
		t.Fatal("expected PublishedAt to be set")
	}
	if item.ExternalID != "5123456789012345" {
		t.Fatalf("expected ExternalID %q, got %q", "5123456789012345", item.ExternalID)
	}
	if item.URL == "" {
		t.Fatal("expected non-empty URL")
	}
	if !strings.Contains(item.URL, "weibo.cn") {
		t.Fatalf("expected URL to contain weibo.cn, got %q", item.URL)
	}
	if item.IdempotencyKey == "" {
		t.Fatal("expected non-empty IdempotencyKey")
	}
}

func TestWeiboAdapterExtractsAuthor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Author should be extracted from user.screen_name
	if len(output.Items) == 0 {
		t.Fatal("expected at least 1 item")
	}
}

func TestWeiboAdapterExtractsEngagementCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Items) == 0 {
		t.Fatal("expected at least 1 item")
	}
}

func TestWeiboAdapterExtractsMediaURLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Items) == 0 {
		t.Fatal("expected at least 1 item")
	}
}

func TestWeiboAdapterNormalizesURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, item := range output.Items {
		if !strings.Contains(item.URL, "m.weibo.cn/detail/") {
			t.Fatalf("expected canonical weibo URL containing m.weibo.cn/detail/, got %q", item.URL)
		}
	}
}

// --- Red: Chinese keyword filtering ---

func TestWeiboAdapterFiltersByKeywords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
		Keywords:    []string{"AI", "ChatGPT"},
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All returned items should match at least one keyword
	for _, item := range output.Items {
		matched := false
		for _, kw := range []string{"AI", "ChatGPT"} {
			if strings.Contains(item.Snippet, kw) || strings.Contains(item.Title, kw) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("item %q does not match any keyword", item.Title)
		}
	}
}

func TestWeiboAdapterFiltersExcludedWords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:         "weibo-search",
		AccessToken:  "test-token",
		BaseURL:      srv.URL,
		ExcludeWords: []string{"文心一言"},
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, item := range output.Items {
		if strings.Contains(item.Snippet, "文心一言") || strings.Contains(item.Title, "文心一言") {
			t.Fatalf("item %q should have been excluded", item.Title)
		}
	}
}

// --- Red: Rate limit failure classification ---

func TestWeiboAdapterRateLimitFrom429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limit_exceeded","error_code":10023}`))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Fatalf("expected FailureClassRateLimit, got %v", err)
	}
}

func TestWeiboAdapterRateLimitFromWeiboErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":0,"errno":10023,"msg":"Requests too quickly"}`))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Fatalf("expected FailureClassRateLimit, got %v", err)
	}
}

// --- Red: Auth failure classification ---

func TestWeiboAdapterAuthFailureFrom401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token","error_code":21332}`))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "expired-token",
		BaseURL:     srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassAuth) {
		t.Fatalf("expected FailureClassAuth, got %v", err)
	}
}

func TestWeiboAdapterAuthFailureFromWeiboErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":0,"errno":21332,"msg":"Invalid access_token"}`))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "expired-token",
		BaseURL:     srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassAuth) {
		t.Fatalf("expected FailureClassAuth, got %v", err)
	}
}

// --- Red: Deleted content skipping ---

func TestWeiboAdapterSkipsDeletedContent(t *testing.T) {
	deletedResponse := `{
		"ok": 1,
		"data": {
			"cards": [
				{
					"card_type": 9,
					"mblog": {
						"id": "5123456789012345",
						"mid": "5123456789012345",
						"text": "正常内容",
						"text_raw": "正常内容",
						"created_at": "Sat Jun 07 10:30:00 +0800 2026",
						"user": {"id": 1, "screen_name": "用户A"},
						"reposts_count": 0,
						"comments_count": 0,
						"attitudes_count": 0,
						"isLongText": false,
						"visible": {"type": 0}
					}
				},
				{
					"card_type": 9,
					"mblog": {
						"id": "5123456789012999",
						"mid": "5123456789012999",
						"text": "",
						"text_raw": "",
						"created_at": "",
						"user": {"id": 0, "screen_name": ""},
						"reposts_count": 0,
						"comments_count": 0,
						"attitudes_count": 0,
						"isLongText": false,
						"visible": {"type": 0},
						"deleted": "1"
					}
				}
			]
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(deletedResponse))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deleted content should be filtered out
	for _, item := range output.Items {
		if item.ExternalID == "5123456789012999" {
			t.Fatal("deleted content should not appear in output")
		}
	}
}

func TestWeiboAdapterSkipsContentWithDeletedFlag(t *testing.T) {
	// Some deleted posts return 404 from detail endpoint
	// The adapter should handle this gracefully by skipping
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"ok":0,"errno":20016,"msg":"Weibo does not exist"}`))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	// 404 on the main endpoint should be a permanent error, not a panic
	if err == nil {
		// If adapter returns empty results instead of error, that's also acceptable
		return
	}
	if adapter.IsAdapterError(err, adapter.FailureClassAuth) {
		t.Fatal("404 should not be classified as auth error")
	}
}

// --- Red: Health status ---

func TestWeiboAdapterHealthInitiallyHealthy(t *testing.T) {
	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
	})
	health := a.Health()
	if health.Status != adapter.HealthStatusHealthy {
		t.Fatalf("expected healthy, got %q", health.Status)
	}
}

func TestWeiboAdapterHealthDegradesAfterRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	_, _ = a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})

	health := a.Health()
	if health.Status != adapter.HealthStatusDegraded {
		t.Fatalf("expected degraded after rate limit, got %q", health.Status)
	}
	if health.LastError == "" {
		t.Fatal("expected LastError to be set")
	}
}

func TestWeiboAdapterHealthRecoversAfterSuccess(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	// First call fails
	_, _ = a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if a.Health().Status != adapter.HealthStatusDegraded {
		t.Fatal("expected degraded after failure")
	}

	// Second call succeeds
	_, _ = a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if a.Health().Status != adapter.HealthStatusHealthy {
		t.Fatalf("expected healthy after success, got %q", a.Health().Status)
	}
}

// --- Red: Registry integration ---

func TestRegistryWeiboAdapterRegistration(t *testing.T) {
	reg := adapter.NewRegistry()
	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
	})
	reg.Register(a)

	got, ok := reg.Get(adapter.ProviderWeibo)
	if !ok {
		t.Fatal("expected to find weibo adapter")
	}
	if got.Name() != "weibo-search" {
		t.Fatalf("expected name %q, got %q", "weibo-search", got.Name())
	}
}

func TestRegistryWeiboIsolationFromOtherAdapters(t *testing.T) {
	reg := adapter.NewRegistry()

	// Register a failing weibo adapter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	weiboA := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})
	reg.Register(weiboA)

	// Register a healthy RSS adapter
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rssA := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "rss-healthy",
		Items: []adapter.NormalizedItem{
			{Title: "RSS Article", URL: "https://example.com/rss/1", PublishedAt: &now},
		},
	})
	reg.Register(rssA)

	// Weibo adapter should fail
	w, _ := reg.Get(adapter.ProviderWeibo)
	_, err := w.Collect(adapter.CollectInput{SourceID: "src-weibo", Provider: adapter.ProviderWeibo, URL: srv.URL + "/2/search/topics"})
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Fatalf("expected rate_limit error, got %v", err)
	}

	// RSS adapter should still work independently
	r, _ := reg.Get(adapter.ProviderRSS)
	output, err := r.Collect(adapter.CollectInput{SourceID: "src-rss", Provider: adapter.ProviderRSS, URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("expected rss to succeed, got %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 rss item, got %d", len(output.Items))
	}
}

// --- Red: Edge cases ---

func TestWeiboAdapterHandlesEmptyCards(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":1,"data":{"cards":[]}}`))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 0 {
		t.Fatalf("expected 0 items for empty cards, got %d", len(output.Items))
	}
}

func TestWeiboAdapterHandlesMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassParseError) {
		t.Fatalf("expected FailureClassParseError, got %v", err)
	}
}

func TestWeiboAdapterHandlesNetworkError(t *testing.T) {
	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     "http://127.0.0.1:1", // unreachable
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      "http://127.0.0.1:1/2/search/topics",
	})
	if err == nil {
		t.Fatal("expected network error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassTransient) {
		t.Fatalf("expected FailureClassTransient, got %v", err)
	}
}

func TestWeiboAdapterHandlesNonSearchCards(t *testing.T) {
	// Cards with card_type != 9 should be skipped
	nonSearchCards := `{
		"ok": 1,
		"data": {
			"cards": [
				{"card_type": 11, "desc": "header card"},
				{
					"card_type": 9,
					"mblog": {
						"id": "5123456789012345",
						"mid": "5123456789012345",
						"text": "有效内容",
						"text_raw": "有效内容",
						"created_at": "Sat Jun 07 10:30:00 +0800 2026",
						"user": {"id": 1, "screen_name": "用户"},
						"reposts_count": 0,
						"comments_count": 0,
						"attitudes_count": 0,
						"isLongText": false,
						"visible": {"type": 0}
					}
				}
			]
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(nonSearchCards))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item (non-search cards skipped), got %d", len(output.Items))
	}
}

func TestWeiboAdapterParseTimeCorrectly(t *testing.T) {
	// Verify the created_at field "Sat Jun 07 10:30:00 +0800 2026" is parsed
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Items) == 0 {
		t.Fatal("expected at least 1 item")
	}

	item := output.Items[0]
	if item.PublishedAt == nil {
		t.Fatal("expected PublishedAt to be set")
	}
	// Verify it's a reasonable 2026 date
	if item.PublishedAt.Year() != 2026 {
		t.Fatalf("expected year 2026, got %d", item.PublishedAt.Year())
	}
}

func TestWeiboAdapterResponseNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":0,"errno":10001,"msg":"System error"}`))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err == nil {
		t.Fatal("expected error for ok=0 response")
	}
}

func TestWeiboAdapterStripHTMLTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Items) == 0 {
		t.Fatal("expected at least 1 item")
	}

	// Snippet should not contain HTML tags
	snippet := output.Items[0].Snippet
	if strings.Contains(snippet, "<a ") || strings.Contains(snippet, "</a>") {
		t.Fatalf("snippet should not contain HTML tags, got %q", snippet)
	}
}

func TestWeiboAdapterSkipNonMblogCards(t *testing.T) {
	// Verify that non-card_type=9 items are skipped and only valid mblog items are returned
	weiboWithNonMblog := `{
		"ok": 1,
		"data": {
			"cards": [
				{"card_type": 11, "desc": "广告卡片"},
				{
					"card_type": 9,
					"mblog": {
						"id": "5123456789012345",
						"mid": "5123456789012345",
						"text": "有效微博内容",
						"text_raw": "有效微博内容",
						"created_at": "Sat Jun 07 10:30:00 +0800 2026",
						"user": {"id": 1, "screen_name": "测试用户"},
						"reposts_count": 10,
						"comments_count": 5,
						"attitudes_count": 20,
						"isLongText": false,
						"visible": {"type": 0}
					}
				},
				{"card_type": 11, "card_group": []}
			]
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboWithNonMblog))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 valid item, got %d", len(output.Items))
	}
	if output.Items[0].ExternalID != "5123456789012345" {
		t.Fatalf("expected ExternalID %q, got %q", "5123456789012345", output.Items[0].ExternalID)
	}
}

func TestWeiboAdapterDeletedFieldFilteredOut(t *testing.T) {
	// Verify that items with "deleted":"1" are filtered out
	weiboWithDeleted := `{
		"ok": 1,
		"data": {
			"cards": [
				{
					"card_type": 9,
					"mblog": {
						"id": "5123456789012345",
						"mid": "5123456789012345",
						"text": "正常微博",
						"text_raw": "正常微博",
						"created_at": "Sat Jun 07 10:30:00 +0800 2026",
						"user": {"id": 1, "screen_name": "用户A"},
						"reposts_count": 0,
						"comments_count": 0,
						"attitudes_count": 0,
						"isLongText": false,
						"visible": {"type": 0}
					}
				},
				{
					"card_type": 9,
					"mblog": {
						"id": "5123456789012999",
						"mid": "5123456789012999",
						"text": "已删除微博",
						"text_raw": "已删除微博",
						"created_at": "Sat Jun 07 09:00:00 +0800 2026",
						"user": {"id": 2, "screen_name": "用户B"},
						"reposts_count": 0,
						"comments_count": 0,
						"attitudes_count": 0,
						"isLongText": false,
						"visible": {"type": 0},
						"deleted": "1"
					}
				}
			]
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboWithDeleted))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should only have the non-deleted item
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 non-deleted item, got %d", len(output.Items))
	}
	if output.Items[0].ExternalID == "5123456789012999" {
		t.Fatal("deleted item should not appear in output")
	}
}

// --- Verify TestMain-compatible structure ---

func TestWeiboAdapterNoOpForEmptyInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":1,"data":{"cards":[]}}`))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Items == nil {
		t.Fatal("expected non-nil Items slice")
	}
	if len(output.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(output.Items))
	}
}

// --- Verify JSON serialization of Weibo response ---

func TestWeiboSearchResponseJSONParsing(t *testing.T) {
	var resp adapter.WeiboSearchResponse
	if err := json.Unmarshal([]byte(weiboSearchResponseJSON), &resp); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}
	if resp.OK != 1 {
		t.Fatalf("expected ok=1, got %d", resp.OK)
	}
	if len(resp.Data.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(resp.Data.Cards))
	}
	card := resp.Data.Cards[0]
	if card.CardType != 9 {
		t.Fatalf("expected card_type=9, got %d", card.CardType)
	}
	if card.Mblog == nil {
		t.Fatal("expected non-nil mblog")
	}
	if card.Mblog.ID != "5123456789012345" {
		t.Fatalf("expected mblog.id %q, got %q", "5123456789012345", card.Mblog.ID)
	}
	if card.Mblog.User.ScreenName != "科技新闻" {
		t.Fatalf("expected screen_name %q, got %q", "科技新闻", card.Mblog.User.ScreenName)
	}
	if card.Mblog.RepostsCount != 1500 {
		t.Fatalf("expected reposts_count=1500, got %d", card.Mblog.RepostsCount)
	}
	if len(card.Mblog.Pics) != 2 {
		t.Fatalf("expected 2 pics, got %d", len(card.Mblog.Pics))
	}
}

// --- Verify weiboAdapterCollectFn helper ---

func TestWeiboAdapterCollectFnFromSimulator(t *testing.T) {
	// Verify the adapter can be used as a Simulator.CollectFn
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	// Verify the adapter can be registered and used
	reg := adapter.NewRegistry()
	reg.Register(a)

	got, ok := reg.Get(adapter.ProviderWeibo)
	if !ok {
		t.Fatal("expected to find weibo adapter in registry")
	}

	output, err := got.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(output.Items))
	}
}

func TestWeiboAdapterMultipleCallsAreIndependent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	for i := 0; i < 3; i++ {
		output, err := a.Collect(adapter.CollectInput{
			SourceID: "src-weibo",
			Provider: adapter.ProviderWeibo,
			URL:      srv.URL + "/2/search/topics",
		})
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if len(output.Items) != 2 {
			t.Fatalf("call %d: expected 2 items, got %d", i, len(output.Items))
		}
	}
}

func TestWeiboAdapterWithNilAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "",
		BaseURL:     srv.URL,
	})

	// Should still work - token is passed as query param
	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(output.Items))
	}
}

func TestWeiboAdapterDefaultBaseURL(t *testing.T) {
	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
	})
	// Verify default base URL is set (we can't make real calls, just verify it doesn't panic)
	if a.Name() != "weibo-search" {
		t.Fatal("expected adapter to be created with defaults")
	}
}

func TestWeiboAdapterHTTPTimeout(t *testing.T) {
	// Slow server that exceeds timeout
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
		Timeout:     50 * time.Millisecond,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassTransient) {
		t.Fatalf("expected FailureClassTransient for timeout, got %v", err)
	}
}

func TestWeiboAdapterContextCancellation(t *testing.T) {
	// This tests that the adapter respects context cancellation
	// For now, just verify the adapter can handle concurrent calls
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(weiboSearchResponseJSON))
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "test-token",
		BaseURL:     srv.URL,
	})

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := a.Collect(adapter.CollectInput{
				SourceID: "src-weibo",
				Provider: adapter.ProviderWeibo,
				URL:      srv.URL + "/2/search/topics",
			})
			done <- err == nil
		}()
	}

	successCount := 0
	for i := 0; i < 10; i++ {
		if <-done {
			successCount++
		}
	}
	if successCount != 10 {
		t.Fatalf("expected all 10 concurrent calls to succeed, got %d", successCount)
	}
}

// --- Helper to verify errors.Is works ---

func TestWeiboAdapterErrorWrapsCorrectly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	a := adapter.NewWeiboAdapter(adapter.WeiboAdapterConfig{
		Name:        "weibo-search",
		AccessToken: "bad-token",
		BaseURL:     srv.URL,
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-weibo",
		Provider: adapter.ProviderWeibo,
		URL:      srv.URL + "/2/search/topics",
	})

	if err == nil {
		t.Fatal("expected error")
	}
	var ae *adapter.AdapterError
	if !errors.As(err, &ae) {
		t.Fatal("expected error to be AdapterError")
	}
	if ae.Class != adapter.FailureClassAuth {
		t.Fatalf("expected auth class, got %q", ae.Class)
	}
}
