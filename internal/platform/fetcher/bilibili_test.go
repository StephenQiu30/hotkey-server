package fetcher_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

// --- Video API helpers ---

func biliVideo(bvid, title, author string, pubdate int64, desc string) map[string]any {
	return map[string]any{
		"bvid":   bvid,
		"title":  title,
		"author": author,
		"pubdate": pubdate,
		"desc":   desc,
		"stat": map[string]any{
			"view":    1000,
			"like":    500,
			"coin":    200,
			"share":   100,
			"reply":   50,
		},
	}
}

func biliVideoListResp(videos []map[string]any) map[string]any {
	return map[string]any{
		"code":    0,
		"message": "0",
		"data": map[string]any{
			"list": map[string]any{
				"vlist": videos,
			},
		},
	}
}

func biliPopularResp(videos []map[string]any) map[string]any {
	return map[string]any{
		"code":    0,
		"message": "0",
		"data": map[string]any{
			"list": videos,
		},
	}
}

func biliDynamicResp(items []map[string]any) map[string]any {
	return map[string]any{
		"code":    0,
		"message": "0",
		"data": map[string]any{
			"items": items,
		},
	}
}

func biliDynamicItem(id, text string, pubdate int64, uname string) map[string]any {
	return map[string]any{
		"id_str": id,
		"type":   "DYNAMIC_TYPE_DRAW",
		"modules": map[string]any{
			"module_dynamic": map[string]any{
				"desc": map[string]any{
					"text": text,
				},
			},
			"module_author": map[string]any{
				"name": uname,
				"pub_ts": pubdate,
			},
		},
	}
}

func biliTakedownResp() map[string]any {
	return map[string]any{
		"code":    -404,
		"message": "视频不见了",
	}
}

func biliRateLimitResp() map[string]any {
	return map[string]any{
		"code":    -412,
		"message": "请求过于频繁",
	}
}

// --- Tests ---

func TestBiliBiliFetcherParsesPopularVideos(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": biliPopularResp([]map[string]any{
			biliVideo("BV1xx411c7mD", "Go语言入门", "UP主A", 1700000000, "Go语言教程"),
			biliVideo("BV1yy411c7mE", "Rust实战", "UP主B", 1700001000, "Rust编程实战"),
		}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err != nil {
		t.Fatalf("fetch bilibili: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}

	item := items[0]
	if item.Title != "Go语言入门" {
		t.Errorf("title = %q, want %q", item.Title, "Go语言入门")
	}
	if item.ExternalID != "BV1xx411c7mD" {
		t.Errorf("externalID = %q, want %q", item.ExternalID, "BV1xx411c7mD")
	}
	if item.URL == "" {
		t.Error("expected non-empty URL")
	}
	if item.Snippet != "Go语言教程" {
		t.Errorf("snippet = %q, want %q", item.Snippet, "Go语言教程")
	}
}

func TestBiliBiliFetcherParsesSpaceVideos(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/space/wbi/arc/search": biliVideoListResp([]map[string]any{
			biliVideo("BV1zz411c7mF", "Kubernetes部署", "UP主C", 1700002000, "K8s教程"),
		}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/space/wbi/arc/search",
	})
	if err != nil {
		t.Fatalf("fetch bilibili space: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Kubernetes部署" {
		t.Errorf("title = %q, want %q", items[0].Title, "Kubernetes部署")
	}
}

func TestBiliBiliFetcherParsesDynamics(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/polymer/web-dynamic/v1/feed/space": biliDynamicResp([]map[string]any{
			biliDynamicItem("12345", "今天天气真好", 1700000000, "UP主A"),
			biliDynamicItem("12346", "分享一个有趣的项目", 1700001000, "UP主B"),
		}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili_dyn",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/polymer/web-dynamic/v1/feed/space",
	})
	if err != nil {
		t.Fatalf("fetch bilibili dynamics: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}
	if items[0].Snippet != "今天天气真好" {
		t.Errorf("snippet = %q, want %q", items[0].Snippet, "今天天气真好")
	}
	if items[0].ExternalID != "dyn_12345" {
		t.Errorf("externalID = %q, want %q", items[0].ExternalID, "dyn_12345")
	}
}

func TestBiliBiliFetcherDeduplicatesByBVID(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": biliPopularResp([]map[string]any{
			biliVideo("BV1xx411c7mD", "Go语言入门", "UP主A", 1700000000, "Go教程"),
			biliVideo("BV1xx411c7mD", "Go语言入门(转载)", "UP主B", 1700001000, "转载"),
		}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err != nil {
		t.Fatalf("fetch bilibili: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 deduplicated item, got %d: %+v", len(items), items)
	}
}

func TestBiliBiliFetcherFiltersTakedowns(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": biliTakedownResp(),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err != nil {
		t.Fatalf("fetch bilibili should not fail on takedown: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items (takedown filtered), got %d", len(items))
	}
}

func TestBiliBiliFetcherHandlesRateLimit(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": biliRateLimitResp(),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err == nil {
		t.Fatal("expected error for rate limit response")
	}
}

func TestBiliBiliFetcherRejectsInvalidSourceType(t *testing.T) {
	f := fetcher.NewBiliBiliFetcher(nil, fetcher.BiliBiliConfig{})
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for non-bilibili source type")
	}
}

func TestBiliBiliFetcherSubtitleFallbackToDesc(t *testing.T) {
	// Video with description but no subtitle — snippet should use desc
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": biliPopularResp([]map[string]any{
			biliVideo("BV1xx411c7mD", "无字幕视频", "UP主A", 1700000000, "这是一段很长的视频描述，应该被截断为摘要"),
		}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err != nil {
		t.Fatalf("fetch bilibili: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Snippet == "" {
		t.Error("expected non-empty snippet from description fallback")
	}
}

func TestBiliBiliFetcherEmptyListReturnsEmpty(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": biliPopularResp([]map[string]any{}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err != nil {
		t.Fatalf("fetch bilibili empty: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items for empty list, got %d", len(items))
	}
}

func TestBiliBiliFetcherSkipsEmptyTitle(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": biliPopularResp([]map[string]any{
			biliVideo("BV1xx411c7mD", "", "UP主A", 1700000000, "有描述无标题"),
			biliVideo("BV1yy411c7mE", "有标题", "UP主B", 1700001000, "正常"),
		}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err != nil {
		t.Fatalf("fetch bilibili: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (empty title skipped), got %d: %+v", len(items), items)
	}
	if items[0].Title != "有标题" {
		t.Errorf("title = %q, want %q", items[0].Title, "有标题")
	}
}

func TestBiliBiliFetcherHandlesHTTPError(t *testing.T) {
	var calls atomic.Int32
	server := &httpServer{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}),
	}
	srv := server.start(t)
	defer srv.Close()

	f := fetcher.NewBiliBiliFetcher(srv.Client(), fetcher.BiliBiliConfig{})
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  srv.URL + "/x/web-interface/popular",
	})
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestBiliBiliFetcherVideoHasCorrectURL(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": biliPopularResp([]map[string]any{
			biliVideo("BV1xx411c7mD", "测试视频", "UP主A", 1700000000, "描述"),
		}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err != nil {
		t.Fatalf("fetch bilibili: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	expectedURL := "https://www.bilibili.com/video/BV1xx411c7mD"
	if items[0].URL != expectedURL {
		t.Errorf("url = %q, want %q", items[0].URL, expectedURL)
	}
}

func TestBiliBiliFetcherDynamicHasCorrectURL(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/polymer/web-dynamic/v1/feed/space": biliDynamicResp([]map[string]any{
			biliDynamicItem("99999", "动态内容", 1700000000, "UP主"),
		}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili_dyn",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/polymer/web-dynamic/v1/feed/space",
	})
	if err != nil {
		t.Fatalf("fetch bilibili dynamics: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	expectedURL := "https://www.bilibili.com/dynamic/99999"
	if items[0].URL != expectedURL {
		t.Errorf("url = %q, want %q", items[0].URL, expectedURL)
	}
}

func TestBiliBiliFetcherHandlesAPIErrorCode(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": map[string]any{
			"code":    -352,
			"message": "请求被拦截",
		},
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err == nil {
		t.Fatal("expected error for non-zero API code")
	}
}

func TestBiliBiliFetcherMultipleVideoStatFields(t *testing.T) {
	server := fakeBiliAPI(t, map[string]any{
		"/x/web-interface/popular": biliPopularResp([]map[string]any{
			{
				"bvid":   "BV1aa411c7mD",
				"title":  "统计测试",
				"author": "UP主",
				"pubdate": 1700000000,
				"desc":   "描述",
				"stat": map[string]any{
					"view":    50000,
					"like":    10000,
					"coin":    5000,
					"share":   2000,
					"reply":   800,
				},
			},
		}),
	})

	f := fetcher.NewBiliBiliFetcher(server.Client(), fetcher.BiliBiliConfig{})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_bili",
		Type: fetcher.SourceTypeBiliBili,
		URL:  server.URL + "/x/web-interface/popular",
	})
	if err != nil {
		t.Fatalf("fetch bilibili: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Score != 50000 {
		t.Errorf("score = %d, want 50000", items[0].Score)
	}
	if items[0].Descendants != 800 {
		t.Errorf("descendants = %d, want 800", items[0].Descendants)
	}
}

// --- helper ---

func fakeBiliAPI(t *testing.T, routes map[string]any) *httptest.Server {
	t.Helper()
	t.Helper()
	mux := http.NewServeMux()
	for path, data := range routes {
		pathCopy := path
		dataCopy := data
		mux.HandleFunc(pathCopy, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(dataCopy)
		})
	}
	return httptest.NewServer(mux)
}
