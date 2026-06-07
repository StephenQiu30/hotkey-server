package adapter

import (
	"context"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

// Fetcher is the interface for fetching items from a source.
type Fetcher interface {
	Fetch(ctx context.Context, source fetcher.Source) ([]fetcher.Item, error)
}

// ArticleFetcher is the interface for extracting article metadata from a URL.
type ArticleFetcher interface {
	FetchArticle(ctx context.Context, url string) fetcher.ArticleMetadata
}

// NewsAdapterConfig configures a NewsAdapter.
type NewsAdapterConfig struct {
	RSSFetcher      Fetcher
	PublicPageFetcher Fetcher
	ArticleFetcher  ArticleFetcher
}

// NewsAdapter bridges fetchers to the adapter.Adapter interface, adding
// article metadata extraction (canonical URL, language, paywall detection).
type NewsAdapter struct {
	rssFetcher       Fetcher
	publicPageFetcher Fetcher
	articleFetcher   ArticleFetcher
}

// NewNewsAdapter creates a new NewsAdapter.
func NewNewsAdapter(cfg NewsAdapterConfig) *NewsAdapter {
	return &NewsAdapter{
		rssFetcher:       cfg.RSSFetcher,
		publicPageFetcher: cfg.PublicPageFetcher,
		articleFetcher:   cfg.ArticleFetcher,
	}
}

func (a *NewsAdapter) Name() string {
	return "news"
}

func (a *NewsAdapter) Provider() Provider {
	return ProviderRSS
}

func (a *NewsAdapter) Collect(input CollectInput) (CollectOutput, error) {
	var items []fetcher.Item
	var err error

	switch input.Provider {
	case ProviderRSS:
		if a.rssFetcher == nil {
			return CollectOutput{}, NewAdapterError(FailureClassPermanent, "rss fetcher not configured", nil)
		}
		items, err = a.rssFetcher.Fetch(context.Background(), fetcher.Source{
			ID:   input.SourceID,
			Type: fetcher.SourceTypeRSS,
			URL:  input.URL,
		})
	case ProviderPublicPage:
		if a.publicPageFetcher == nil {
			return CollectOutput{}, NewAdapterError(FailureClassPermanent, "public page fetcher not configured", nil)
		}
		items, err = a.publicPageFetcher.Fetch(context.Background(), fetcher.Source{
			ID:   input.SourceID,
			Type: fetcher.SourceTypePublicPage,
			URL:  input.URL,
		})
	default:
		return CollectOutput{}, NewAdapterError(FailureClassPermanent, "unsupported provider: "+string(input.Provider), nil)
	}

	if err != nil {
		return CollectOutput{}, NewAdapterError(FailureClassTransient, "fetch failed", err)
	}

	normalized := make([]NormalizedItem, 0, len(items))
	for _, item := range items {
		ni := a.normalizeItem(input.SourceID, item)
		normalized = append(normalized, ni)
	}

	return CollectOutput{Items: normalized}, nil
}

func (a *NewsAdapter) Health() HealthInfo {
	return HealthInfo{
		Status:        HealthStatusHealthy,
		LastCheckedAt: time.Now().UTC(),
	}
}

func (a *NewsAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsIncremental: true,
		MaxItemsPerFetch:    100,
		RateLimitPerHour:    60,
	}
}

func (a *NewsAdapter) normalizeItem(sourceID string, item fetcher.Item) NormalizedItem {
	ni := NormalizedItem{
		Title:       strings.TrimSpace(item.Title),
		URL:         strings.TrimSpace(item.URL),
		ExternalID:  strings.TrimSpace(item.ExternalID),
		PublishedAt: item.PublishedAt,
		IdempotencyKey: NewIdempotencyKey(sourceID, item.URL),
	}

	// Enrich with article metadata if available
	if a.articleFetcher != nil && item.URL != "" {
		article := a.articleFetcher.FetchArticle(context.Background(), item.URL)

		// Use canonical URL as snippet for downstream dedup
		if article.CanonicalURL != "" && article.CanonicalURL != item.URL {
			ni.Snippet = article.CanonicalURL
		}

		if article.Language != "" {
			ni.Language = article.Language
		}

		if article.PaywallDetected {
			ni.MetadataOnly = true
			ni.Snippet = prependMetadataOnly(ni.Snippet)
		}

		// Use article published time if fetcher didn't provide one
		if ni.PublishedAt == nil && article.PublishedAt != nil {
			ni.PublishedAt = article.PublishedAt
		}
	}

	return ni
}

const metadataOnlyPrefix = "[metadata_only] "

func prependMetadataOnly(snippet string) string {
	if strings.HasPrefix(snippet, metadataOnlyPrefix) {
		return snippet
	}
	return metadataOnlyPrefix + snippet
}

// IsMetadataOnly checks if a NormalizedItem snippet indicates metadata-only content.
func IsMetadataOnly(snippet string) bool {
	return strings.HasPrefix(snippet, metadataOnlyPrefix)
}
