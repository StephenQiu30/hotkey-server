package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultCommentSampleLimit = 5

type HNConfig struct {
	CommentSampleLimit int
}

type HNFetcher struct {
	client             *http.Client
	commentSampleLimit int
}

func NewHNFetcher(client *http.Client, cfg HNConfig) *HNFetcher {
	limit := cfg.CommentSampleLimit
	if limit <= 0 {
		limit = defaultCommentSampleLimit
	}
	return &HNFetcher{
		client:             httpClient(client),
		commentSampleLimit: limit,
	}
}

func (f *HNFetcher) CommentSampleLimit() int {
	return f.commentSampleLimit
}

func (f *HNFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	if source.Type != SourceTypeHackerNews {
		return nil, errors.New("hackernews fetcher requires hackernews source")
	}

	baseURL := hnBaseURL(source.URL)

	ids, err := f.fetchStoryIDs(ctx, source.URL)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []Item{}, nil
	}

	seen := make(map[string]struct{})
	items := make([]Item, 0, len(ids))
	for _, id := range ids {
		item, err := f.fetchItem(ctx, baseURL, id)
		if err != nil {
			continue
		}
		if item.Deleted || item.Dead {
			continue
		}
		if item.Type != "story" || item.Title == "" {
			continue
		}
		if item.URL == "" {
			item.URL = fmt.Sprintf("https://news.ycombinator.com/item?id=%d", item.ID)
		}
		if _, exists := seen[item.URL]; exists {
			continue
		}
		seen[item.URL] = struct{}{}

		published := time.Unix(item.Time, 0).UTC()
		fetched := Item{
			Title:       strings.TrimSpace(item.Title),
			URL:         strings.TrimSpace(item.URL),
			ExternalID:  strconv.FormatInt(item.ID, 10),
			PublishedAt: &published,
			Score:       item.Score,
			Descendants: item.Descendants,
		}
		if len(item.Kids) > 0 {
			fetched.CommentSamples = f.fetchCommentSamples(ctx, baseURL, item.Kids)
		}
		items = append(items, fetched)
	}
	return items, nil
}

func (f *HNFetcher) fetchStoryIDs(ctx context.Context, url string) ([]int64, error) {
	body, err := fetchBody(ctx, f.client, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var ids []int64
	if err := json.NewDecoder(body).Decode(&ids); err != nil {
		return nil, fmt.Errorf("parse story ids: %w", err)
	}
	return ids, nil
}

func (f *HNFetcher) fetchItem(ctx context.Context, baseURL string, id int64) (hnItem, error) {
	itemURL := fmt.Sprintf("%sitem/%d.json", baseURL, id)
	body, err := fetchBody(ctx, f.client, itemURL)
	if err != nil {
		return hnItem{}, err
	}
	defer body.Close()

	var item hnItem
	if err := json.NewDecoder(body).Decode(&item); err != nil {
		return hnItem{}, fmt.Errorf("parse item %d: %w", id, err)
	}
	return item, nil
}

func (f *HNFetcher) fetchCommentSamples(ctx context.Context, baseURL string, kidIDs []int64) []CommentSample {
	limit := f.commentSampleLimit
	if len(kidIDs) > limit {
		kidIDs = kidIDs[:limit]
	}
	samples := make([]CommentSample, 0, len(kidIDs))
	for _, kidID := range kidIDs {
		item, err := f.fetchItem(ctx, baseURL, kidID)
		if err != nil {
			continue
		}
		if item.Deleted || item.Dead || item.Text == "" {
			continue
		}
		samples = append(samples, CommentSample{
			Text:   strings.TrimSpace(item.Text),
			Author: strings.TrimSpace(item.By),
		})
	}
	return samples
}

// hnBaseURL derives the item API base from a story-list URL.
// "https://hacker-news.firebaseio.com/v0/topstories.json" → "https://hacker-news.firebaseio.com/v0/"
func hnBaseURL(storyURL string) string {
	parsed, err := url.Parse(storyURL)
	if err != nil {
		return storyURL
	}
	// Strip the filename (e.g. "topstories.json") to get the directory
	idx := strings.LastIndex(parsed.Path, "/")
	if idx >= 0 {
		parsed.Path = parsed.Path[:idx+1]
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

type hnItem struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	By          string  `json:"by"`
	Score       int     `json:"score"`
	Descendants int     `json:"descendants"`
	Time        int64   `json:"time"`
	Type        string  `json:"type"`
	Deleted     bool    `json:"deleted"`
	Dead        bool    `json:"dead"`
	Kids        []int64 `json:"kids"`
	Text        string  `json:"text"`
}
