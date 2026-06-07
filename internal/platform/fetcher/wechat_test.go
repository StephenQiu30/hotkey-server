package fetcher_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

func TestWeChatMPFetcherParsesArticlePage(t *testing.T) {
	articleHTML := `<!DOCTYPE html>
<html>
<head>
<meta property="og:title" content="AI 改变世界" />
<meta property="og:image" content="https://mmbiz.qpic.cn/cover.jpg" />
<meta property="og:description" content="人工智能正在改变我们的生活。" />
<meta property="article:published_time" content="2026-05-30T10:00:00Z" />
<meta name="author" content="科技前沿" />
<title>AI 改变世界</title>
</head>
<body>
<div id="js_content">
<p>人工智能正在深刻改变我们的生活方式。</p>
<p>从自动驾驶到智能医疗，AI 技术无处不在。</p>
</div>
</body>
</html>`

	server := fakeHTTPServer(articleHTML)
	f := fetcher.NewWeChatMPFetcher(server.Client())
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:             "src_wx",
		Type:           fetcher.SourceTypeWeChatMP,
		URL:            server.URL,
		ComplianceNote: "Only collect publicly available WeChat articles.",
	})
	if err != nil {
		t.Fatalf("fetch wechat: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.Title != "AI 改变世界" {
		t.Errorf("title = %q, want %q", item.Title, "AI 改变世界")
	}
	if item.Author != "科技前沿" {
		t.Errorf("author = %q, want %q", item.Author, "科技前沿")
	}
	if item.Snippet == "" {
		t.Error("expected non-empty snippet from article body")
	}
	if item.CoverImageURL != "https://mmbiz.qpic.cn/cover.jpg" {
		t.Errorf("coverImageURL = %q, want %q", item.CoverImageURL, "https://mmbiz.qpic.cn/cover.jpg")
	}
	if item.PublishedAt == nil {
		t.Error("expected published_at to be set")
	}
}

func TestWeChatMPFetcherReturnsErrorOnHTTPFailure(t *testing.T) {
	server := &httpServer{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	}
	srv := server.start(t)
	defer srv.Close()

	f := fetcher.NewWeChatMPFetcher(srv.Client())
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:             "src_wx",
		Type:           fetcher.SourceTypeWeChatMP,
		URL:            srv.URL,
		ComplianceNote: "Public article only.",
	})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestWeChatMPFetcherRejectsNonWeChatSourceType(t *testing.T) {
	f := fetcher.NewWeChatMPFetcher(nil)
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for non-wechat source type")
	}
}

func TestWeChatMPFetcherRequiresComplianceNote(t *testing.T) {
	f := fetcher.NewWeChatMPFetcher(nil)
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_wx",
		Type: fetcher.SourceTypeWeChatMP,
		URL:  "https://mp.weixin.qq.com/s/abc123",
	})
	if err == nil {
		t.Fatal("expected error for missing compliance note")
	}
}

func TestWeChatMPFetcherExtractsTitleFromH1WhenNoOGTitle(t *testing.T) {
	articleHTML := `<!DOCTYPE html>
<html>
<head><title>Fallback Title</title></head>
<body>
<h1 class="rich_media_title">从标题标签提取</h1>
<div id="js_content"><p>正文内容。</p></div>
</body>
</html>`

	server := fakeHTTPServer(articleHTML)
	f := fetcher.NewWeChatMPFetcher(server.Client())
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:             "src_wx",
		Type:           fetcher.SourceTypeWeChatMP,
		URL:            server.URL,
		ComplianceNote: "Public article only.",
	})
	if err != nil {
		t.Fatalf("fetch wechat: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "从标题标签提取" {
		t.Errorf("title = %q, want %q", items[0].Title, "从标题标签提取")
	}
}
