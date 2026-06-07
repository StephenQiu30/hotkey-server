package fetcher_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

func TestExtractCanonicalURLFromLinkTag(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
<link rel="canonical" href="https://example.com/canonical-article" />
<title>Test Article</title>
</head>
<body><p>Content</p></body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/redirect")
	if article.CanonicalURL != "https://example.com/canonical-article" {
		t.Fatalf("expected canonical URL %q, got %q", "https://example.com/canonical-article", article.CanonicalURL)
	}
}

func TestExtractCanonicalURLFallsBackToProvidedURL(t *testing.T) {
	html := `<!DOCTYPE html>
<html><head><title>No Canonical</title></head><body>Content</body></html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/fallback")
	if article.CanonicalURL != "https://example.com/fallback" {
		t.Fatalf("expected fallback URL %q, got %q", "https://example.com/fallback", article.CanonicalURL)
	}
}

func TestExtractLanguageFromHTMLLangAttribute(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head><title>中文文章</title></head>
<body><p>内容</p></body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/zh")
	if article.Language != "zh" {
		t.Fatalf("expected language %q, got %q", "zh", article.Language)
	}
}

func TestExtractLanguageFromMetaContentLanguage(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
<meta http-equiv="content-language" content="ja" />
<title>日本語記事</title>
</head>
<body><p>コンテンツ</p></body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/ja")
	if article.Language != "ja" {
		t.Fatalf("expected language %q, got %q", "ja", article.Language)
	}
}

func TestExtractLanguageDefaultsToEmptyWhenMissing(t *testing.T) {
	html := `<!DOCTYPE html>
<html><head><title>No Language</title></head><body>Content</body></html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/no-lang")
	if article.Language != "" {
		t.Fatalf("expected empty language, got %q", article.Language)
	}
}

func TestExtractPublishedTimeFromArticleMeta(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
<meta property="article:published_time" content="2026-05-31T10:30:00Z" />
<title>Published Article</title>
</head>
<body><p>Content</p></body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/published")
	want := time.Date(2026, 5, 31, 10, 30, 0, 0, time.UTC)
	if article.PublishedAt == nil || !article.PublishedAt.Equal(want) {
		t.Fatalf("expected published_at %s, got %v", want.Format(time.RFC3339), article.PublishedAt)
	}
}

func TestExtractPublishedTimeFromTimeTag(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="en">
<head><title>Time Tag Article</title></head>
<body><time datetime="2026-06-01T08:00:00+09:00">June 1, 2026</time></body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/time-tag")
	if article.PublishedAt == nil {
		t.Fatal("expected published_at to be set from <time> tag")
	}
	if article.PublishedAt.Year() != 2026 || article.PublishedAt.Month() != time.June || article.PublishedAt.Day() != 1 {
		t.Fatalf("unexpected published_at: %v", article.PublishedAt)
	}
}

func TestExtractPublishedTimeDefaultsToNil(t *testing.T) {
	html := `<!DOCTYPE html>
<html><head><title>No Time</title></head><body>Content</body></html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/no-time")
	if article.PublishedAt != nil {
		t.Fatalf("expected nil published_at, got %v", article.PublishedAt)
	}
}

func TestExtractDescriptionFromMetaTag(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
<meta name="description" content="This is a test article description for extraction." />
<title>Desc Article</title>
</head>
<body><p>Content</p></body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/desc")
	if article.Description != "This is a test article description for extraction." {
		t.Fatalf("expected description %q, got %q", "This is a test article description for extraction.", article.Description)
	}
}

func TestExtractDescriptionFromOGDescription(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
<meta property="og:description" content="OpenGraph description text." />
<title>OG Desc</title>
</head>
<body><p>Content</p></body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/og-desc")
	if article.Description != "OpenGraph description text." {
		t.Fatalf("expected og:description %q, got %q", "OpenGraph description text.", article.Description)
	}
}

func TestDetectPaywallFromGenericIndicators(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="en">
<head><title>Premium Article</title></head>
<body>
<article>
<p>Free intro paragraph.</p>
<div class="paywall">Subscribe to continue reading this premium content.</div>
</article>
</body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/paywall")
	if !article.PaywallDetected {
		t.Fatal("expected paywall to be detected")
	}
}

func TestDetectPaywallFromSubscriberOnlyText(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="en">
<head><title>Subscriber Article</title></head>
<body>
<p>This article is available only to subscribers. Please log in to read more.</p>
</body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/subscriber-only")
	if !article.PaywallDetected {
		t.Fatal("expected paywall to be detected from subscriber-only text")
	}
}

func TestNoPaywallForFreeContent(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="en">
<head><title>Free Article</title></head>
<body>
<article><p>This is a completely free article with no paywall indicators.</p></article>
</body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://example.com/free")
	if article.PaywallDetected {
		t.Fatal("expected no paywall for free content")
	}
}

func TestExtractAllFieldsFromCompleteArticle(t *testing.T) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<link rel="canonical" href="https://news.example.com/ai-breakthrough" />
<meta property="article:published_time" content="2026-06-07T00:00:00Z" />
<meta name="description" content="AI 突破性进展的详细报道。" />
<title>AI 突破性进展</title>
</head>
<body>
<article><p>详细内容。</p></article>
</body>
</html>`

	article := fetcher.ExtractArticleMetadata(html, "https://news.example.com/ai-breakthrough?ref=rss")
	if article.CanonicalURL != "https://news.example.com/ai-breakthrough" {
		t.Fatalf("canonical URL: got %q", article.CanonicalURL)
	}
	if article.Language != "zh" {
		t.Fatalf("language: got %q", article.Language)
	}
	if article.PublishedAt == nil || article.PublishedAt.Year() != 2026 {
		t.Fatalf("published_at: got %v", article.PublishedAt)
	}
	if article.Description != "AI 突破性进展的详细报道。" {
		t.Fatalf("description: got %q", article.Description)
	}
	if article.PaywallDetected {
		t.Fatal("expected no paywall")
	}
}
