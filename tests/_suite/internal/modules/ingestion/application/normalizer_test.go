package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

func TestNormalizeCapturedItemCleansAndPreservesCapturedFacts(t *testing.T) {
	t.Parallel()

	publishedAt := time.Date(2026, time.July, 15, 14, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	observedAt := time.Date(2026, time.July, 16, 9, 45, 0, 0, time.UTC)
	viewCount := int64(0)
	likeCount := int64(7)
	item := domain.CapturedItem{
		Version:     domain.CapturedItemVersionV2,
		SourceCode:  "rss",
		ExternalID:  "  feed-entry-42  ",
		ContentType: "article",
		Title:       " Cafe\u0301 <script>discard()</script> Headline\x00 ",
		Body:        "<p>Hello&nbsp;<strong>world</strong>.</p><style>.hidden {}</style><script>alert(1)</script><img src=\"pixel.gif\">\x07",
		Language:    " en ",
		URL:         "HTTPS://EXAMPLE.COM:443/news/item/?utm_source=feed&b=2&fbclid=ignored#read",
		Author:      "  A\u0300LICE  ",
		PublishedAt: &publishedAt,
		ObservedAt:  observedAt,
		Metrics: domain.SourceMetrics{
			ViewCount: &viewCount,
			LikeCount: &likeCount,
		},
	}

	content, err := NormalizeCapturedItem(item, 42)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem() error = %v", err)
	}

	if content.SourceConnectionID != 42 || content.ExternalID != "feed-entry-42" || content.ContentType != "article" {
		t.Fatalf("content identity = %#v", content)
	}
	if content.Title != "Café Headline" || content.Body != "Hello world." || content.Excerpt != "Hello world." {
		t.Fatalf("content text = %#v, want NFC-cleaned captured title/body/excerpt", content)
	}
	if content.CanonicalURL != "https://example.com/news/item?b=2" || content.Language != "en" {
		t.Fatalf("canonical URL/language = %q / %q", content.CanonicalURL, content.Language)
	}
	if content.PublishedAt != publishedAt.UTC() || content.FetchedAt != observedAt {
		t.Fatalf("content timestamps = published:%s fetched:%s", content.PublishedAt, content.FetchedAt)
	}
	if content.Author.DisplayName != "ÀLICE" || content.Author.ExternalID != authorID(42, "àlice") {
		t.Fatalf("content author = %#v", content.Author)
	}
	wantHash := sha256.Sum256([]byte("Café Headline\x00Hello world."))
	if content.ContentHash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("content hash = %q, want stable normalized title/body hash", content.ContentHash)
	}
	if content.Metrics.ViewCount == nil || *content.Metrics.ViewCount != 0 || content.Metrics.LikeCount == nil || *content.Metrics.LikeCount != 7 || content.Metrics.CommentCount != nil || content.Metrics.ShareCount != nil {
		t.Fatalf("content metrics = %#v, want nil distinct from explicit zero", content.Metrics)
	}

	viewCount = 99
	likeCount = 88
	if *content.Metrics.ViewCount != 0 || *content.Metrics.LikeCount != 7 {
		t.Fatalf("normalized metrics changed after source mutation: %#v", content.Metrics)
	}
}

func TestNormalizeCapturedItemUsesObservedAtAndMapsHackerNewsComment(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.July, 16, 10, 0, 0, 0, time.UTC)
	content, err := NormalizeCapturedItem(domain.CapturedItem{
		Version:     domain.CapturedItemVersionV2,
		SourceCode:  "hacker_news",
		ExternalID:  "102",
		ContentType: "comment",
		Body:        "A retained comment",
		URL:         "https://news.ycombinator.com/item?id=102",
		ObservedAt:  observedAt,
	}, 9)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem() error = %v", err)
	}
	if content.ContentType != "post" || content.PublishedAt != observedAt || content.FetchedAt != observedAt {
		t.Fatalf("content = %#v, want HN post with observed timestamp fallback", content)
	}
}

func TestNormalizeCapturedItemCanonicalizesIPv6Host(t *testing.T) {
	t.Parallel()

	content, err := NormalizeCapturedItem(capturedItemForNormalization("HTTP://[2001:DB8::1]:80/news/?utm_campaign=fixture", "IPv6", ""), 8)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem() error = %v", err)
	}
	if content.CanonicalURL != "http://[2001:db8::1]/news" {
		t.Fatalf("canonical IPv6 URL = %q", content.CanonicalURL)
	}
}

func TestNormalizeCapturedItemPreservesOpaqueExternalID(t *testing.T) {
	t.Parallel()

	firstItem := capturedItemForNormalization("https://example.test/opaque-one", "first", "")
	firstItem.ExternalID = "  a<b>  "
	secondItem := capturedItemForNormalization("https://example.test/opaque-two", "second", "")
	secondItem.ExternalID = "a"

	first, err := NormalizeCapturedItem(firstItem, 3)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem(first) error = %v", err)
	}
	second, err := NormalizeCapturedItem(secondItem, 3)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem(second) error = %v", err)
	}
	if first.ExternalID != "a<b>" || second.ExternalID != "a" || first.ExternalID == second.ExternalID {
		t.Fatalf("opaque external IDs = %q / %q, want distinct trim-only source identities", first.ExternalID, second.ExternalID)
	}
}

func TestNormalizeCapturedItemTokenizesHTMLWithoutDiscardingComparisonText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantBody string
	}{
		{name: "ordinary_comparison_and_entity", body: "A < B > C<p>visible&nbsp;text</p>", wantBody: "A < B > C visible text"},
		{name: "unclosed_script", body: "before<script>discard <b>all remaining", wantBody: "before"},
		{name: "nested_script_subtree", body: "before<script><style>discard</style></script><p>after</p>", wantBody: "before after"},
		{name: "unclosed_style", body: "before<style>discard <script>all remaining", wantBody: "before"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := capturedItemForNormalization("https://example.test/html-"+test.name, "Cafe&#x301;", "")
			item.Body = test.body + "\x00"
			content, err := NormalizeCapturedItem(item, 4)
			if err != nil {
				t.Fatalf("NormalizeCapturedItem() error = %v", err)
			}
			if content.Title != "Café" || content.Body != test.wantBody {
				t.Fatalf("normalized title/body = %q / %q, want NFC title and %q", content.Title, content.Body, test.wantBody)
			}
		})
	}
}

func TestNormalizeCapturedItemCanonicalizesIDNAndTrailingHostDot(t *testing.T) {
	t.Parallel()

	content, err := NormalizeCapturedItem(capturedItemForNormalization("https://BÜCHER.example./story", "IDN", ""), 8)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem() error = %v", err)
	}
	if content.CanonicalURL != "https://xn--bcher-kva.example/story" {
		t.Fatalf("canonical IDN URL = %q", content.CanonicalURL)
	}
}

func TestNormalizeCapturedItemRejectsInvalidCapturedFactsWithStableCodes(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.July, 16, 10, 0, 0, 0, time.UTC)
	base := domain.CapturedItem{
		Version:     domain.CapturedItemVersionV2,
		SourceCode:  "rss",
		ExternalID:  "item-1",
		ContentType: "article",
		Title:       "valid title",
		URL:         "https://example.test/item-1",
		ObservedAt:  observedAt,
	}

	tests := []struct {
		name     string
		item     domain.CapturedItem
		sourceID int64
		want     ingestiondomain.ErrorCode
	}{
		{name: "missing_content", item: replaceCaptured(base, func(item *domain.CapturedItem) { item.Title = "" }), sourceID: 1, want: ingestiondomain.ErrorCodeEmptyContent},
		{name: "invalid_url", item: replaceCaptured(base, func(item *domain.CapturedItem) { item.URL = "ftp://example.test/item-1" }), sourceID: 1, want: ingestiondomain.ErrorCodeInvalidCanonicalURL},
		{name: "unsupported_type", item: replaceCaptured(base, func(item *domain.CapturedItem) { item.ContentType = "comment" }), sourceID: 1, want: ingestiondomain.ErrorCodeInvalidContentType},
		{name: "invalid_source", item: base, sourceID: 0, want: ingestiondomain.ErrorCodeInvalidCapturedItem},
		{name: "negative_metric", item: replaceCaptured(base, func(item *domain.CapturedItem) { value := int64(-1); item.Metrics.ViewCount = &value }), sourceID: 1, want: ingestiondomain.ErrorCodeInvalidCapturedItem},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NormalizeCapturedItem(test.item, test.sourceID)
			if err == nil {
				t.Fatal("NormalizeCapturedItem() error = nil")
			}
			var domainErr *ingestiondomain.Error
			if !errors.As(err, &domainErr) || domainErr.Code != test.want {
				t.Fatalf("NormalizeCapturedItem() error = %v, want stable code %q", err, test.want)
			}
		})
	}
}

func TestNormalizeCapturedItemScopesAuthorIdentityBySource(t *testing.T) {
	t.Parallel()

	item := capturedItemForNormalization("https://example.test/author", "Alicia", "Alice")
	first, err := NormalizeCapturedItem(item, 11)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem(first) error = %v", err)
	}
	item.Author = "  ALICE "
	second, err := NormalizeCapturedItem(item, 11)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem(second) error = %v", err)
	}
	third, err := NormalizeCapturedItem(item, 12)
	if err != nil {
		t.Fatalf("NormalizeCapturedItem(third) error = %v", err)
	}
	if first.Author.ExternalID != second.Author.ExternalID || first.Author.ExternalID == third.Author.ExternalID {
		t.Fatalf("author IDs = %q / %q / %q, want stable same-source and distinct cross-source", first.Author.ExternalID, second.Author.ExternalID, third.Author.ExternalID)
	}
}

func capturedItemForNormalization(rawURL, title, author string) domain.CapturedItem {
	return domain.CapturedItem{
		Version:     domain.CapturedItemVersionV2,
		SourceCode:  "rss",
		ExternalID:  "item-1",
		ContentType: "article",
		Title:       title,
		Body:        "body",
		URL:         rawURL,
		Author:      author,
		ObservedAt:  time.Date(2026, time.July, 16, 10, 0, 0, 0, time.UTC),
	}
}

func replaceCaptured(item domain.CapturedItem, change func(*domain.CapturedItem)) domain.CapturedItem {
	change(&item)
	return item
}

func authorID(sourceID int64, normalizedAuthor string) string {
	hash := sha256.Sum256([]byte("source:" + strconv.FormatInt(sourceID, 10) + "\x00" + normalizedAuthor))
	return hex.EncodeToString(hash[:])
}
