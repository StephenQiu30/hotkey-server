package adapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

func TestNewsAdapterRSSProvider(t *testing.T) {
	a := adapter.NewNewsAdapter(adapter.NewsAdapterConfig{
		RSSFetcher: stubFetcher([]fetcher.Item{
			{Title: "AI News", URL: "https://example.com/ai", ExternalID: "ai-1", PublishedAt: ptrTime(time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC))},
		}),
	})

	if a.Name() != "news" {
		t.Fatalf("expected name %q, got %q", "news", a.Name())
	}
	if a.Provider() != adapter.ProviderRSS {
		t.Fatalf("expected provider %q, got %q", adapter.ProviderRSS, a.Provider())
	}

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-1",
		Provider: adapter.ProviderRSS,
		URL:      "https://example.com/rss.xml",
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}
	item := output.Items[0]
	if item.Title != "AI News" {
		t.Fatalf("title: got %q", item.Title)
	}
	if item.URL != "https://example.com/ai" {
		t.Fatalf("url: got %q", item.URL)
	}
	if item.ExternalID != "ai-1" {
		t.Fatalf("externalID: got %q", item.ExternalID)
	}
	if item.IdempotencyKey == "" {
		t.Fatal("expected non-empty idempotency key")
	}
}

func TestNewsAdapterPublicPageProvider(t *testing.T) {
	a := adapter.NewNewsAdapter(adapter.NewsAdapterConfig{
		PublicPageFetcher: stubFetcher([]fetcher.Item{
			{Title: "Public Page", URL: "https://example.com/page", ExternalID: "https://example.com/page"},
		}),
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-2",
		Provider: adapter.ProviderPublicPage,
		URL:      "https://example.com/page",
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}
	if output.Items[0].Title != "Public Page" {
		t.Fatalf("title: got %q", output.Items[0].Title)
	}
}

func TestNewsAdapterUnsupportedProvider(t *testing.T) {
	a := adapter.NewNewsAdapter(adapter.NewsAdapterConfig{})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-3",
		Provider: adapter.ProviderOfficialAPI,
		URL:      "https://example.com/api",
	})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	var ae *adapter.AdapterError
	if !errors.As(err, &ae) || ae.Class != adapter.FailureClassPermanent {
		t.Fatalf("expected permanent adapter error, got %v", err)
	}
}

func TestNewsAdapterFetcherError(t *testing.T) {
	a := adapter.NewNewsAdapter(adapter.NewsAdapterConfig{
		RSSFetcher: errFetcher(errors.New("network timeout")),
	})

	_, err := a.Collect(adapter.CollectInput{
		SourceID: "src-4",
		Provider: adapter.ProviderRSS,
		URL:      "https://example.com/rss.xml",
	})
	if err == nil {
		t.Fatal("expected fetcher error to propagate")
	}
	var ae *adapter.AdapterError
	if !errors.As(err, &ae) || ae.Class != adapter.FailureClassTransient {
		t.Fatalf("expected transient adapter error, got %v", err)
	}
}

func TestNewsAdapterMetadataOnlyForPaywall(t *testing.T) {
	a := adapter.NewNewsAdapter(adapter.NewsAdapterConfig{
		RSSFetcher: stubFetcher([]fetcher.Item{
			{Title: "Free Article", URL: "https://example.com/free", ExternalID: "free-1"},
		}),
		ArticleFetcher: stubArticleFetcher(map[string]fetcher.ArticleMetadata{
			"https://example.com/free": {CanonicalURL: "https://example.com/free", Language: "en", PaywallDetected: true},
		}),
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-5",
		Provider: adapter.ProviderRSS,
		URL:      "https://example.com/rss.xml",
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}
	if !output.Items[0].MetadataOnly {
		t.Fatal("expected MetadataOnly=true for paywall content")
	}
}

func TestNewsAdapterFreeContentNotMetadataOnly(t *testing.T) {
	a := adapter.NewNewsAdapter(adapter.NewsAdapterConfig{
		RSSFetcher: stubFetcher([]fetcher.Item{
			{Title: "Free Article", URL: "https://example.com/free", ExternalID: "free-1"},
		}),
		ArticleFetcher: stubArticleFetcher(map[string]fetcher.ArticleMetadata{
			"https://example.com/free": {CanonicalURL: "https://example.com/free", Language: "en", PaywallDetected: false},
		}),
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-6",
		Provider: adapter.ProviderRSS,
		URL:      "https://example.com/rss.xml",
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if output.Items[0].MetadataOnly {
		t.Fatal("expected MetadataOnly=false for free content")
	}
}

func TestNewsAdapterUsesCanonicalURLFromArticle(t *testing.T) {
	a := adapter.NewNewsAdapter(adapter.NewsAdapterConfig{
		RSSFetcher: stubFetcher([]fetcher.Item{
			{Title: "Redirected", URL: "https://example.com/redirect?id=1", ExternalID: "redir-1"},
		}),
		ArticleFetcher: stubArticleFetcher(map[string]fetcher.ArticleMetadata{
			"https://example.com/redirect?id=1": {CanonicalURL: "https://example.com/canonical", Language: "en"},
		}),
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-7",
		Provider: adapter.ProviderRSS,
		URL:      "https://example.com/rss.xml",
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	// URL in NormalizedItem should remain the original (for idempotency), but snippet carries canonical info
	if output.Items[0].URL != "https://example.com/redirect?id=1" {
		t.Fatalf("URL should remain original, got %q", output.Items[0].URL)
	}
	if output.Items[0].Snippet != "https://example.com/canonical" {
		t.Fatalf("expected snippet to carry canonical URL, got %q", output.Items[0].Snippet)
	}
}

func TestNewsAdapterLanguageFromArticleMetadata(t *testing.T) {
	a := adapter.NewNewsAdapter(adapter.NewsAdapterConfig{
		RSSFetcher: stubFetcher([]fetcher.Item{
			{Title: "中文文章", URL: "https://example.com/zh", ExternalID: "zh-1"},
		}),
		ArticleFetcher: stubArticleFetcher(map[string]fetcher.ArticleMetadata{
			"https://example.com/zh": {CanonicalURL: "https://example.com/zh", Language: "zh"},
		}),
	})

	output, err := a.Collect(adapter.CollectInput{
		SourceID: "src-8",
		Provider: adapter.ProviderRSS,
		URL:      "https://example.com/rss.xml",
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if output.Items[0].Language != "zh" {
		t.Fatalf("expected language %q, got %q", "zh", output.Items[0].Language)
	}
}

func TestNewsAdapterHealthAndCapabilities(t *testing.T) {
	a := adapter.NewNewsAdapter(adapter.NewsAdapterConfig{
		RSSFetcher: stubFetcher(nil),
	})

	health := a.Health()
	if health.Status != adapter.HealthStatusHealthy {
		t.Fatalf("expected healthy, got %s", health.Status)
	}

	caps := a.Capabilities()
	if !caps.SupportsIncremental {
		t.Fatal("expected incremental support")
	}
}

// --- stubs ---

type stubFetcherImpl struct {
	items []fetcher.Item
	err   error
}

func (f *stubFetcherImpl) Fetch(_ context.Context, _ fetcher.Source) ([]fetcher.Item, error) {
	return f.items, f.err
}

func stubFetcher(items []fetcher.Item) *stubFetcherImpl {
	return &stubFetcherImpl{items: items}
}

func errFetcher(err error) *stubFetcherImpl {
	return &stubFetcherImpl{err: err}
}

type stubArticleFetcherImpl struct {
	metadata map[string]fetcher.ArticleMetadata
}

func (f *stubArticleFetcherImpl) FetchArticle(_ context.Context, url string) fetcher.ArticleMetadata {
	if m, ok := f.metadata[url]; ok {
		return m
	}
	return fetcher.ArticleMetadata{CanonicalURL: url}
}

func stubArticleFetcher(metadata map[string]fetcher.ArticleMetadata) *stubArticleFetcherImpl {
	return &stubArticleFetcherImpl{metadata: metadata}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
