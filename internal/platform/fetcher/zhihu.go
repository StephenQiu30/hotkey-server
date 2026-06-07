package fetcher

import (
	"context"
	"errors"
	"net/http"
)

// ZhihuFetcher fetches content from Zhihu platform.
// Currently a stub that returns empty results; real API integration pending.
type ZhihuFetcher struct {
	client *http.Client
}

// NewZhihuFetcher creates a new ZhihuFetcher.
func NewZhihuFetcher(client *http.Client) *ZhihuFetcher {
	return &ZhihuFetcher{client: httpClient(client)}
}

func (f *ZhihuFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	if source.Type != SourceTypeZhihu {
		return nil, errors.New("zhihu fetcher requires zhihu source")
	}
	// TODO: implement real Zhihu API integration with OAuth
	return []Item{}, nil
}
