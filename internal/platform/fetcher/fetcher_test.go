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
