package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultMaxCommentSamples = 10

// RedditConfig holds configuration for the Reddit fetcher.
type RedditConfig struct {
	AllowNSFW         bool
	MaxCommentSamples int
}

// RedditOption is a functional option for NewRedditFetcher.
type RedditOption func(*RedditConfig)

// RedditFetcher fetches posts from Reddit's public JSON API.
type RedditFetcher struct {
	client *http.Client
	config RedditConfig
}

// NewRedditFetcher creates a RedditFetcher with optional configuration.
func NewRedditFetcher(client *http.Client, opts ...RedditOption) *RedditFetcher {
	cfg := RedditConfig{
		MaxCommentSamples: defaultMaxCommentSamples,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &RedditFetcher{
		client: httpClient(client),
		config: cfg,
	}
}

// Fetch retrieves posts from a Reddit subreddit listing via the public JSON API.
func (f *RedditFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	if source.Type != SourceTypeReddit {
		return nil, errors.New("reddit fetcher requires reddit source")
	}

	body, err := fetchBody(ctx, f.client, source.URL)
	if err != nil {
		return nil, classifyHTTPError(err)
	}
	defer func() {
		if closeErr := body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close response body: %w", closeErr)
		}
	}()

	payload, err := io.ReadAll(io.LimitReader(body, 10<<20))
	if err != nil {
		return nil, err
	}

	posts, comments, err := parseRedditListing(payload)
	if err != nil {
		return nil, fmt.Errorf("parse reddit: %w", err)
	}

	seen := make(map[string]struct{})
	items := make([]Item, 0, len(posts))
	for _, post := range posts {
		if shouldSkipPost(post, f.config.AllowNSFW) {
			continue
		}
		item := mapPostToItem(post)
		// URL deduplication
		if _, exists := seen[item.URL]; exists {
			continue
		}
		seen[item.URL] = struct{}{}
		item.CommentSamples = filterCommentsForPost(comments, f.config.MaxCommentSamples)
		items = append(items, item)
	}
	return items, nil
}

func classifyHTTPError(err error) error {
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "status 429"):
		return ErrRateLimited
	case strings.Contains(errStr, "status 403"):
		return ErrForbidden
	case strings.Contains(errStr, "status 404"):
		return ErrNotFound
	default:
		return err
	}
}

type redditPost struct {
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Permalink  string  `json:"permalink"`
	Author     string  `json:"author"`
	Subreddit  string  `json:"subreddit"`
	CreatedUTC float64 `json:"created_utc"`
	Selftext   string  `json:"selftext"`
	Over18     bool    `json:"over_18"`
	Name       string  `json:"name"`
	IsSelf     bool    `json:"is_self"`
	Score      int     `json:"score"`
	NumComments int    `json:"num_comments"`
}

type redditComment struct {
	Author     string  `json:"author"`
	Body       string  `json:"body"`
	CreatedUTC float64 `json:"created_utc"`
	Name       string  `json:"name"`
}

type redditListingResponse []json.RawMessage

type redditListing struct {
	Data struct {
		Children []struct {
			Kind string          `json:"kind"`
			Data json.RawMessage `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func parseRedditListing(payload []byte) ([]redditPost, []redditComment, error) {
	var arr redditListingResponse
	if err := json.Unmarshal(payload, &arr); err != nil {
		// Try single listing format (no comments)
		var single redditListing
		if err2 := json.Unmarshal(payload, &single); err2 != nil {
			return nil, nil, fmt.Errorf("unmarshal listing: %w", err)
		}
		posts := parsePostsFromListing(single)
		return posts, nil, nil
	}

	if len(arr) == 0 {
		return nil, nil, nil
	}

	// First element is the post listing
	var postListing redditListing
	if err := json.Unmarshal(arr[0], &postListing); err != nil {
		return nil, nil, fmt.Errorf("unmarshal post listing: %w", err)
	}
	posts := parsePostsFromListing(postListing)

	// Second element (if present) is the comment listing
	var comments []redditComment
	if len(arr) > 1 {
		var commentListing redditListing
		if err := json.Unmarshal(arr[1], &commentListing); err == nil {
			for _, child := range commentListing.Data.Children {
				if child.Kind != "t1" {
					continue
				}
				var c redditComment
				if err := json.Unmarshal(child.Data, &c); err == nil {
					comments = append(comments, c)
				}
			}
		}
	}

	return posts, comments, nil
}

func parsePostsFromListing(listing redditListing) []redditPost {
	posts := make([]redditPost, 0, len(listing.Data.Children))
	for _, child := range listing.Data.Children {
		if child.Kind != "t3" {
			continue
		}
		var p redditPost
		if err := json.Unmarshal(child.Data, &p); err != nil {
			continue
		}
		posts = append(posts, p)
	}
	return posts
}

func shouldSkipPost(post redditPost, allowNSFW bool) bool {
	if post.Author == "[deleted]" || post.Author == "[removed]" {
		return true
	}
	if post.Over18 && !allowNSFW {
		return true
	}
	return false
}

func mapPostToItem(post redditPost) Item {
	item := Item{
		Title:      strings.TrimSpace(post.Title),
		ExternalID: post.Name,
		Score:      post.Score,
		Descendants: post.NumComments,
	}
	if post.IsSelf || post.URL == "" {
		// Self-post: use permalink from API
		if post.Permalink != "" {
			item.URL = "https://www.reddit.com" + post.Permalink
		} else {
			item.URL = fmt.Sprintf("https://www.reddit.com/r/%s/comments/%s/", post.Subreddit, strings.TrimPrefix(post.Name, "t3_"))
		}
	} else {
		item.URL = strings.TrimSpace(post.URL)
	}
	if post.CreatedUTC > 0 {
		t := time.Unix(int64(post.CreatedUTC), 0).UTC()
		item.PublishedAt = &t
	}
	if post.Selftext != "" {
		item.Snippet = strings.TrimSpace(post.Selftext)
	}
	return item
}

func filterCommentsForPost(comments []redditComment, maxSamples int) []CommentSample {
	if len(comments) == 0 {
		return nil
	}
	result := make([]CommentSample, 0, len(comments))
	for _, c := range comments {
		if c.Body == "[deleted]" || c.Body == "[removed]" || c.Author == "[deleted]" {
			continue
		}
		sample := CommentSample{
			Text:   strings.TrimSpace(c.Body),
			Author: strings.TrimSpace(c.Author),
		}
		result = append(result, sample)
	}
	if len(result) > maxSamples {
		result = result[:maxSamples]
	}
	return result
}
