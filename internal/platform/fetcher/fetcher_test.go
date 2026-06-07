package fetcher_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

func TestRSSFetcherParsesItemsFromFakeHTTPServer(t *testing.T) {
	server := fakeHTTPServer(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>AI Feed</title>
<item><title>Model Launch</title><link>https://example.com/model</link><guid>model-1</guid><pubDate>Sun, 31 May 2026 01:00:00 GMT</pubDate></item>
</channel></rss>`)

	items, err := fetcher.NewRSSFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("fetch rss: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one parsed rss item, got %#v", items)
	}
	item := items[0]
	if item.Title != "Model Launch" {
		t.Fatalf("expected rss title %q, got %q", "Model Launch", item.Title)
	}
	if item.URL != "https://example.com/model" {
		t.Fatalf("expected rss URL %q, got %q", "https://example.com/model", item.URL)
	}
	if item.ExternalID != "model-1" {
		t.Fatalf("expected rss external ID %q, got %q", "model-1", item.ExternalID)
	}
	wantPublishedAt := time.Date(2026, 5, 31, 1, 0, 0, 0, time.UTC)
	if item.PublishedAt == nil || !item.PublishedAt.Equal(wantPublishedAt) {
		t.Fatalf("expected rss published_at %s, got %v", wantPublishedAt.Format(time.RFC3339), item.PublishedAt)
	}
}

func TestPublicPageFetcherReturnsPageBoundaryWithoutPrivateCollection(t *testing.T) {
	server := fakeHTTPServer(`<html><head><title>Public AI Page</title></head><body>public content</body></html>`)

	items, err := fetcher.NewPublicPageFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:             "src_page",
		Type:           fetcher.SourceTypePublicPage,
		URL:            server.URL,
		ComplianceNote: "Public page only; no login or anti-bot bypass.",
	})
	if err != nil {
		t.Fatalf("fetch public page: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one public page boundary item, got %#v", items)
	}
	item := items[0]
	if item.Title != "Public AI Page" || item.URL != server.URL || item.ExternalID != server.URL {
		t.Fatalf("expected public page boundary item with title, URL, and external ID from source URL; got %#v", item)
	}
}

func TestRSSFetcherReturnsBodyCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       errReadCloser{Reader: bytes.NewBufferString(`<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`), closeErr: closeErr},
			Request:    req,
		}, nil
	})}

	_, err := fetcher.NewRSSFetcher(client).Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  "https://fake.local/rss.xml",
	})
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected response body close error, got %v", err)
	}
}

func TestXiaohongshuFetcherRejectsWrongSourceType(t *testing.T) {
	f := fetcher.NewXiaohongshuFetcher(nil)
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:             "src_xhs",
		Type:           fetcher.SourceTypeRSS,
		URL:            "https://www.xiaohongshu.com/explore/note-1",
		ComplianceNote: "test",
	})
	if err == nil {
		t.Fatal("expected error for wrong source type")
	}
}

func TestXiaohongshuFetcherRequiresComplianceNote(t *testing.T) {
	f := fetcher.NewXiaohongshuFetcher(nil)
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_xhs",
		Type: fetcher.SourceTypeXiaohongshu,
		URL:  "https://www.xiaohongshu.com/explore/note-1",
	})
	if err == nil {
		t.Fatal("expected error for missing compliance note")
	}
}

func TestXiaohongshuFetcherReturnsPageTitleOnSuccess(t *testing.T) {
	server := fakeHTTPServer(`<html><head><title>小红书笔记标题</title></head><body>content</body></html>`)

	items, err := fetcher.NewXiaohongshuFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:             "src_xhs",
		Type:           fetcher.SourceTypeXiaohongshu,
		URL:            server.URL,
		ComplianceNote: "Only collect publicly visible notes.",
	})
	if err != nil {
		t.Fatalf("fetch xiaohongshu: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if items[0].Title != "小红书笔记标题" {
		t.Fatalf("expected title %q, got %q", "小红书笔记标题", items[0].Title)
	}
}

func TestXiaohongshuFetcherReturnsBodyCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       errReadCloser{Reader: bytes.NewBufferString(`<html><head><title>XHS</title></head></html>`), closeErr: closeErr},
			Request:    req,
		}, nil
	})}

	_, err := fetcher.NewXiaohongshuFetcher(client).Fetch(context.Background(), fetcher.Source{
		ID:             "src_xhs",
		Type:           fetcher.SourceTypeXiaohongshu,
		URL:            "https://fake.local/xhs",
		ComplianceNote: "Public notes only.",
	})
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected response body close error, got %v", err)
	}
}

func TestPublicPageFetcherReturnsBodyCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       errReadCloser{Reader: bytes.NewBufferString(`<html><head><title>Public AI Page</title></head></html>`), closeErr: closeErr},
			Request:    req,
		}, nil
	})}

	_, err := fetcher.NewPublicPageFetcher(client).Fetch(context.Background(), fetcher.Source{
		ID:             "src_page",
		Type:           fetcher.SourceTypePublicPage,
		URL:            "https://fake.local/page",
		ComplianceNote: "Public page only.",
	})
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected response body close error, got %v", err)
	}
}

type fakeServer struct {
	URL  string
	body string
}

func fakeHTTPServer(body string) fakeServer {
	return fakeServer{URL: "https://fake.local/source", body: body}
}

func (s fakeServer) Client() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(s.body)),
			Request:    req,
		}, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReadCloser struct {
	io.Reader
	closeErr error
}

func (c errReadCloser) Close() error {
	return c.closeErr
}

func TestRSSFetcherExtractsSnippetAndAuthor(t *testing.T) {
	server := fakeHTTPServer(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>AI Feed</title>
<item>
  <title>Model Launch</title>
  <link>https://example.com/model</link>
  <guid>model-1</guid>
  <pubDate>Sun, 31 May 2026 01:00:00 GMT</pubDate>
  <description>A new AI model with 1T parameters.</description>
  <author>editor@example.com (AI Editor)</author>
</item>
</channel></rss>`)

	items, err := fetcher.NewRSSFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("fetch rss: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	item := items[0]
	if item.Snippet != "A new AI model with 1T parameters." {
		t.Errorf("snippet = %q, want %q", item.Snippet, "A new AI model with 1T parameters.")
	}
	if item.Author != "editor@example.com (AI Editor)" {
		t.Errorf("author = %q, want %q", item.Author, "editor@example.com (AI Editor)")
	}
}

func TestRSSFetcherPrefersContentEncodedOverDescription(t *testing.T) {
	server := fakeHTTPServer(`<?xml version="1.0"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel>
<title>Blog</title>
<item>
  <title>Deep Dive</title>
  <link>https://example.com/deep</link>
  <guid>deep-1</guid>
  <description>Short summary</description>
  <content:encoded><![CDATA[<p>Full HTML content with <b>rich</b> formatting.</p>]]></content:encoded>
</item>
</channel></rss>`)

	items, err := fetcher.NewRSSFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("fetch rss: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if items[0].Snippet != "<p>Full HTML content with <b>rich</b> formatting.</p>" {
		t.Errorf("snippet = %q, want content:encoded value", items[0].Snippet)
	}
}

func TestRSSFetcherExtractsMediaThumbnailAsCover(t *testing.T) {
	server := fakeHTTPServer(`<?xml version="1.0"?>
<rss version="2.0" xmlns:media="http://search.yahoo.com/mrss/"><channel>
<title>News</title>
<item>
  <title>Breaking</title>
  <link>https://example.com/breaking</link>
  <guid>break-1</guid>
  <media:thumbnail url="https://img.example.com/cover.jpg" />
</item>
</channel></rss>`)

	items, err := fetcher.NewRSSFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("fetch rss: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if items[0].CoverImageURL != "https://img.example.com/cover.jpg" {
		t.Errorf("coverImageURL = %q, want %q", items[0].CoverImageURL, "https://img.example.com/cover.jpg")
	}
}

func TestRSSFetcherExtractsEnclosureAsCover(t *testing.T) {
	server := fakeHTTPServer(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Photo Feed</title>
<item>
  <title>Sunset</title>
  <link>https://example.com/sunset</link>
  <guid>sunset-1</guid>
  <enclosure url="https://img.example.com/sunset.jpg" length="12345" type="image/jpeg" />
</item>
</channel></rss>`)

	items, err := fetcher.NewRSSFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("fetch rss: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if items[0].CoverImageURL != "https://img.example.com/sunset.jpg" {
		t.Errorf("coverImageURL = %q, want %q", items[0].CoverImageURL, "https://img.example.com/sunset.jpg")
	}
}

func TestAtomFetcherParsesEntries(t *testing.T) {
	server := fakeHTTPServer(`<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<title>Tech Feed</title>
<entry>
  <title>Go 1.25 Released</title>
  <link href="https://go.dev/blog/go1.25" />
  <id>urn:uuid:12345</id>
  <updated>2026-05-30T10:00:00Z</updated>
  <author><name>Go Team</name></author>
  <summary>Go 1.25 adds generics improvements.</summary>
</entry>
<entry>
  <title>Another Post</title>
  <link href="https://example.com/another" />
  <id>urn:uuid:67890</id>
  <updated>2026-05-29T08:00:00Z</updated>
</entry>
</feed>`)

	items, err := fetcher.NewRSSFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_atom",
		Type: fetcher.SourceTypeRSS,
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("fetch atom: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 atom entries, got %d: %+v", len(items), items)
	}
	entry := items[0]
	if entry.Title != "Go 1.25 Released" {
		t.Errorf("title = %q, want %q", entry.Title, "Go 1.25 Released")
	}
	if entry.URL != "https://go.dev/blog/go1.25" {
		t.Errorf("url = %q, want %q", entry.URL, "https://go.dev/blog/go1.25")
	}
	if entry.ExternalID != "urn:uuid:12345" {
		t.Errorf("externalID = %q, want %q", entry.ExternalID, "urn:uuid:12345")
	}
	if entry.Author != "Go Team" {
		t.Errorf("author = %q, want %q", entry.Author, "Go Team")
	}
	if entry.Snippet != "Go 1.25 adds generics improvements." {
		t.Errorf("snippet = %q, want %q", entry.Snippet, "Go 1.25 adds generics improvements.")
	}
	if entry.PublishedAt == nil {
		t.Fatal("expected published_at to be set")
	}
}

func TestRSSFetcherReturnsErrorForInvalidXML(t *testing.T) {
	server := fakeHTTPServer(`this is not xml at all <><><`)

	_, err := fetcher.NewRSSFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  server.URL,
	})
	if err == nil {
		t.Fatal("expected parse error for invalid XML")
	}
}

func TestRSSFetcherSkipsItemsWithoutTitleOrLink(t *testing.T) {
	server := fakeHTTPServer(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Feed</title>
<item><title></title><link></link></item>
<item><title>Valid</title><link>https://example.com/valid</link></item>
</channel></rss>`)

	items, err := fetcher.NewRSSFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("fetch rss: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 valid item, got %d", len(items))
	}
	if items[0].Title != "Valid" {
		t.Errorf("title = %q, want %q", items[0].Title, "Valid")
	}
}

func TestMultiFetcherDispatchesBySourceType(t *testing.T) {
	rssServer := fakeHTTPServer(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>RSS Feed</title>
<item><title>RSS Item</title><link>https://example.com/rss</link></item>
</channel></rss>`)
	pageServer := fakeHTTPServer(`<html><head><title>Page Title</title></head><body></body></html>`)

	multi := fetcher.NewMultiFetcher(map[fetcher.SourceType]fetcher.Fetcher{
		fetcher.SourceTypeRSS:        fetcher.NewRSSFetcher(rssServer.Client()),
		fetcher.SourceTypePublicPage: fetcher.NewPublicPageFetcher(pageServer.Client()),
	})

	// Dispatch to RSS
	items, err := multi.Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  rssServer.URL,
	})
	if err != nil {
		t.Fatalf("multi fetch rss: %v", err)
	}
	if len(items) != 1 || items[0].Title != "RSS Item" {
		t.Fatalf("expected RSS item, got %+v", items)
	}

	// Dispatch to PublicPage
	items, err = multi.Fetch(context.Background(), fetcher.Source{
		ID:             "src_page",
		Type:           fetcher.SourceTypePublicPage,
		URL:            pageServer.URL,
		ComplianceNote: "Public page only.",
	})
	if err != nil {
		t.Fatalf("multi fetch page: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Page Title" {
		t.Fatalf("expected page item, got %+v", items)
	}
}

func TestMultiFetcherReturnsErrorForUnknownSourceType(t *testing.T) {
	multi := fetcher.NewMultiFetcher(map[fetcher.SourceType]fetcher.Fetcher{
		fetcher.SourceTypeRSS: fetcher.NewRSSFetcher(nil),
	})

	_, err := multi.Fetch(context.Background(), fetcher.Source{
		ID:   "src_unknown",
		Type: fetcher.SourceType("unknown_type"),
		URL:  "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for unknown source type")
	}
}

func TestMultiFetcherReturnsErrorForNilFetcher(t *testing.T) {
	multi := fetcher.NewMultiFetcher(map[fetcher.SourceType]fetcher.Fetcher{
		fetcher.SourceTypeRSS: nil,
	})

	_, err := multi.Fetch(context.Background(), fetcher.Source{
		ID:   "src_nil",
		Type: fetcher.SourceTypeRSS,
		URL:  "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for nil fetcher entry")
	}
}
