package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultRedditCommentLimit = 10

// RedditError classifies Reddit API failures.
type RedditError struct {
	Class   string // "rate_limit", "auth", "not_found", "transient"
	Message string
}

func (e *RedditError) Error() string {
	return fmt.Sprintf("reddit: [%s] %s", e.Class, e.Message)
}

// RedditConfig holds configuration for the Reddit fetcher.
type RedditConfig struct {
	CommentSampleLimit int
	AllowNSFW          bool
}

// RedditFetcher fetches posts from Reddit's public JSON API.
type RedditFetcher struct {
	client             *http.Client
	commentSampleLimit int
	allowNSFW          bool
}

func NewRedditFetcher(client *http.Client, cfg RedditConfig) *RedditFetcher {
	limit := cfg.CommentSampleLimit
	if limit <= 0 {
		limit = defaultRedditCommentLimit
	}
	return &RedditFetcher{
		client:             httpClient(client),
		commentSampleLimit: limit,
		allowNSFW:          cfg.AllowNSFW,
	}
}

func (f *RedditFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	if source.Type != SourceTypeReddit {
		return nil, errors.New("reddit fetcher requires reddit source")
	}

	body, err := fetchBody(ctx, f.client, source.URL)
	if err != nil {
		return nil, f.classifyHTTPError(err, source.URL)
	}
	defer body.Close()

	var listing redditListing
	if err := json.NewDecoder(body).Decode(&listing); err != nil {
		return nil, fmt.Errorf("parse reddit listing: %w", err)
	}

	seen := make(map[string]struct{})
	items := make([]Item, 0, len(listing.Data.Children))
	for _, child := range listing.Data.Children {
		post := child.Data

		// Skip deleted/removed posts
		if post.Author == "[deleted]" || post.Author == "[removed]" {
			continue
		}
		if post.Title == "[deleted]" || post.Title == "[removed]" {
			continue
		}

		// Skip NSFW if not allowed
		if post.Over18 && !f.allowNSFW {
			continue
		}

		// Use permalink as URL if no external URL
		url := strings.TrimSpace(post.URL)
		if url == "" {
			url = fmt.Sprintf("https://www.reddit.com%s", post.Permalink)
		}

		// Deduplicate by URL
		if _, exists := seen[url]; exists {
			continue
		}
		seen[url] = struct{}{}

		published := time.Unix(int64(post.CreatedUTC), 0).UTC()
		item := Item{
			Title:       strings.TrimSpace(post.Title),
			URL:         url,
			ExternalID:  post.ID,
			PublishedAt: &published,
			Score:       post.Score,
			Descendants: post.NumComments,
		}

		// Fetch comment samples for posts with comments
		if post.NumComments > 0 {
			item.CommentSamples = f.fetchComments(ctx, source.URL, post.ID)
		}

		items = append(items, item)
	}
	return items, nil
}

func (f *RedditFetcher) fetchComments(ctx context.Context, listingURL string, postID string) []CommentSample {
	// Derive comment URL from listing URL
	// e.g. "/r/golang/hot.json" -> "/r/golang/comments/{postID}.json"
	commentURL := commentEndpoint(listingURL, postID)

	body, err := fetchBody(ctx, f.client, commentURL)
	if err != nil {
		return nil
	}
	defer body.Close()

	var resp []json.RawMessage
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil
	}
	if len(resp) < 2 {
		return nil
	}

	var commentListing redditListing
	if err := json.Unmarshal(resp[1], &commentListing); err != nil {
		return nil
	}

	limit := f.commentSampleLimit
	samples := make([]CommentSample, 0, limit)
	for _, child := range commentListing.Data.Children {
		if len(samples) >= limit {
			break
		}
		comment := child.Data
		if comment.Body == "" || comment.Author == "[deleted]" || comment.Author == "[removed]" {
			continue
		}
		samples = append(samples, CommentSample{
			Text:   strings.TrimSpace(comment.Body),
			Author: strings.TrimSpace(comment.Author),
		})
	}
	return samples
}

func (f *RedditFetcher) classifyHTTPError(err error, url string) error {
	// Check if it's an HTTP status error from fetchBody
	errMsg := err.Error()
	if strings.Contains(errMsg, "status 429") {
		return &RedditError{Class: "rate_limit", Message: "too many requests"}
	}
	if strings.Contains(errMsg, "status 403") || strings.Contains(errMsg, "status 401") {
		return &RedditError{Class: "auth", Message: "forbidden or private subreddit"}
	}
	if strings.Contains(errMsg, "status 404") {
		return &RedditError{Class: "not_found", Message: "subreddit not found"}
	}
	return &RedditError{Class: "transient", Message: err.Error()}
}

// commentEndpoint derives the comment API URL from a listing URL.
// "/r/golang/hot.json" + "abc123" -> "/r/golang/comments/abc123.json"
func commentEndpoint(listingURL, postID string) string {
	// Find the subreddit path
	idx := strings.Index(listingURL, "/r/")
	if idx < 0 {
		return listingURL
	}
	subPath := listingURL[idx:]
	// Strip everything after /r/{name}
	slashIdx := strings.Index(subPath[3:], "/")
	if slashIdx >= 0 {
		subPath = subPath[:3+slashIdx]
	}
	base := listingURL[:idx]
	return fmt.Sprintf("%s%s/comments/%s.json", base, subPath, postID)
}

// Reddit API response types

type redditListing struct {
	Data redditListingData `json:"data"`
}

type redditListingData struct {
	Children []redditChild `json:"children"`
}

type redditChild struct {
	Data redditPost `json:"data"`
}

type redditPost struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Author      string  `json:"author"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
	Subreddit   string  `json:"subreddit"`
	SelfText    string  `json:"self_text"`
	Over18      bool    `json:"over_18"`
	Permalink   string  `json:"permalink"`
	Body        string  `json:"body"`   // for comments
}
