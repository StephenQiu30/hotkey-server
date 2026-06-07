package fetcher_test

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

func TestMultiFetcherDispatchesBySourceType(t *testing.T) {
	rssItems := []fetcher.Item{{Title: "RSS Item", URL: "https://example.com/rss"}}
	ppItems := []fetcher.Item{{Title: "Page Title", URL: "https://example.com/page"}}

	multi := fetcher.NewMultiFetcher(map[fetcher.SourceType]fetcher.Fetcher{
		fetcher.SourceTypeRSS:        &stubFetcher{items: rssItems},
		fetcher.SourceTypePublicPage: &stubFetcher{items: ppItems},
	})

	ctx := context.Background()

	got, err := multi.Fetch(ctx, fetcher.Source{Type: fetcher.SourceTypeRSS, URL: "https://example.com/rss.xml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Title != "RSS Item" {
		t.Fatalf("expected RSS item, got %+v", got)
	}

	got, err = multi.Fetch(ctx, fetcher.Source{Type: fetcher.SourceTypePublicPage, URL: "https://example.com/page"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Page Title" {
		t.Fatalf("expected page item, got %+v", got)
	}
}

func TestMultiFetcherReturnsErrorForUnregisteredType(t *testing.T) {
	multi := fetcher.NewMultiFetcher(map[fetcher.SourceType]fetcher.Fetcher{
		fetcher.SourceTypeRSS: &stubFetcher{},
	})

	_, err := multi.Fetch(context.Background(), fetcher.Source{Type: fetcher.SourceTypePublicPage, URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for unregistered source type")
	}
}

func TestMultiFetcherPropagatesFetcherError(t *testing.T) {
	expected := errors.New("network timeout")
	multi := fetcher.NewMultiFetcher(map[fetcher.SourceType]fetcher.Fetcher{
		fetcher.SourceTypeRSS: &stubFetcher{err: expected},
	})

	_, err := multi.Fetch(context.Background(), fetcher.Source{Type: fetcher.SourceTypeRSS, URL: "https://example.com/rss.xml"})
	if !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

type stubFetcher struct {
	items []fetcher.Item
	err   error
}

func (f *stubFetcher) Fetch(context.Context, fetcher.Source) ([]fetcher.Item, error) {
	return f.items, f.err
}
