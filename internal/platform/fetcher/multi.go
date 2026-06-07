package fetcher

import (
	"context"
	"fmt"
)

// MultiFetcher dispatches Fetch calls to the registered fetcher for the source type.
type MultiFetcher struct {
	fetchers map[SourceType]Fetcher
}

// NewMultiFetcher creates a MultiFetcher from a map of source type to fetcher.
func NewMultiFetcher(fetchers map[SourceType]Fetcher) *MultiFetcher {
	clone := make(map[SourceType]Fetcher, len(fetchers))
	for k, v := range fetchers {
		clone[k] = v
	}
	return &MultiFetcher{fetchers: clone}
}

// Fetch dispatches to the fetcher registered for source.Type.
func (m *MultiFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	f, ok := m.fetchers[source.Type]
	if !ok {
		return nil, fmt.Errorf("no fetcher registered for source type %q", source.Type)
	}
	return f.Fetch(ctx, source)
}
