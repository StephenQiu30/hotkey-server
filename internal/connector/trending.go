package connector

import "context"

// TrendingCollector defines the interface for platforms that expose
// a trending/hot list (e.g., Weibo hot search, Zhihu hot list, Baidu trending).
type TrendingCollector interface {
	// FetchTrending returns the current trending/hot list from the platform.
	FetchTrending(ctx context.Context) ([]TrendingItem, error)

	// Name returns the platform name (e.g., "weibo", "zhihu", "baidu").
	Name() string
}
