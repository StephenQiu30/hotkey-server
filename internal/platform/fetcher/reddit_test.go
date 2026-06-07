package fetcher_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

// ---------------------------------------------------------------------------
// Happy-path tests
// ---------------------------------------------------------------------------

func TestRedditFetcherParsesSelfPostFromListing(t *testing.T) {
	body := redditListingJSON([]redditTestPost{{
		Title:      "Go 1.25 released",
		URL:        "https://www.reddit.com/r/golang/comments/abc123/go_125_released/",
		Permalink:  "/r/golang/comments/abc123/go_125_released/",
		Author:     "gopher",
		Subreddit:  "golang",
		CreatedUTC: 1748649600.0,
		SelfText:   "Go 1.25 includes exciting features.",
		Over18:     false,
		FullName:   "t3_abc123",
		IsSelf:     true,
		Score:      42,
	}}, nil)

	server := fakeHTTPServer(body)
	items, err := fetcher.NewRedditFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/golang",
	})
	if err != nil {
		t.Fatalf("fetch reddit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.Title != "Go 1.25 released" {
		t.Fatalf("expected title %q, got %q", "Go 1.25 released", item.Title)
	}
	// Self-post URL should use permalink from API
	wantURL := "https://www.reddit.com/r/golang/comments/abc123/go_125_released/"
	if item.URL != wantURL {
		t.Fatalf("expected URL %q, got %q", wantURL, item.URL)
	}
	if item.ExternalID != "t3_abc123" {
		t.Fatalf("expected external ID %q, got %q", "t3_abc123", item.ExternalID)
	}
	wantPublished := time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC)
	if item.PublishedAt == nil || !item.PublishedAt.Equal(wantPublished) {
		t.Fatalf("expected published_at %s, got %v", wantPublished, item.PublishedAt)
	}
	if item.Score != 42 {
		t.Fatalf("expected score 42, got %d", item.Score)
	}
}

func TestRedditFetcherParsesLinkPostWithURL(t *testing.T) {
	body := redditListingJSON([]redditTestPost{{
		Title:      "New AI paper",
		URL:        "https://arxiv.org/abs/2501.00001",
		Permalink:  "/r/MachineLearning/comments/def456/new_ai_paper/",
		Author:     "researcher",
		Subreddit:  "MachineLearning",
		CreatedUTC: 1748649600.0,
		SelfText:   "",
		Over18:     false,
		FullName:   "t3_def456",
		IsSelf:     false,
	}}, nil)

	server := fakeHTTPServer(body)
	items, err := fetcher.NewRedditFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/MachineLearning",
	})
	if err != nil {
		t.Fatalf("fetch reddit link post: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.URL != "https://arxiv.org/abs/2501.00001" {
		t.Fatalf("expected external URL, got %q", item.URL)
	}
	if item.ExternalID != "t3_def456" {
		t.Fatalf("expected external ID %q, got %q", "t3_def456", item.ExternalID)
	}
}

func TestRedditFetcherIncludesCommentSamples(t *testing.T) {
	comments := []redditTestComment{
		{Author: "user1", Body: "Great post!", CreatedUTC: 1748649700.0, FullName: "t1_cmt1"},
		{Author: "user2", Body: "Thanks for sharing.", CreatedUTC: 1748649800.0, FullName: "t1_cmt2"},
	}
	body := redditListingJSON([]redditTestPost{{
		Title:      "Discussion thread",
		URL:        "https://www.reddit.com/r/golang/comments/xyz/discussion/",
		Permalink:  "/r/golang/comments/xyz/discussion/",
		Author:     "gopher",
		Subreddit:  "golang",
		CreatedUTC: 1748649600.0,
		SelfText:   "Let's discuss.",
		Over18:     false,
		FullName:   "t3_xyz",
		IsSelf:     true,
	}}, comments)

	server := fakeHTTPServer(body)
	items, err := fetcher.NewRedditFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/golang",
	})
	if err != nil {
		t.Fatalf("fetch reddit with comments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if len(items[0].CommentSamples) != 2 {
		t.Fatalf("expected 2 comment samples, got %d", len(items[0].CommentSamples))
	}
	if items[0].CommentSamples[0].Author != "user1" || items[0].CommentSamples[0].Text != "Great post!" {
		t.Fatalf("unexpected first comment: %+v", items[0].CommentSamples[0])
	}
}

// ---------------------------------------------------------------------------
// Deleted / removed post tests
// ---------------------------------------------------------------------------

func TestRedditFetcherSkipsDeletedAuthorPosts(t *testing.T) {
	body := redditListingJSON([]redditTestPost{
		{Title: "Deleted post", URL: "https://www.reddit.com/r/golang/comments/del1/", Permalink: "/r/golang/comments/del1/", Author: "[deleted]", Subreddit: "golang", CreatedUTC: 1748649600.0, FullName: "t3_del1"},
		{Title: "Valid post", URL: "https://www.reddit.com/r/golang/comments/val1/", Permalink: "/r/golang/comments/val1/", Author: "gopher", Subreddit: "golang", CreatedUTC: 1748649600.0, FullName: "t3_val1"},
	}, nil)

	server := fakeHTTPServer(body)
	items, err := fetcher.NewRedditFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/golang",
	})
	if err != nil {
		t.Fatalf("fetch reddit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (deleted skipped), got %d", len(items))
	}
	if items[0].ExternalID != "t3_val1" {
		t.Fatalf("expected valid post, got %q", items[0].ExternalID)
	}
}

func TestRedditFetcherSkipsRemovedPosts(t *testing.T) {
	body := redditListingJSON([]redditTestPost{
		{Title: "Removed post", URL: "https://www.reddit.com/r/golang/comments/rem1/", Permalink: "/r/golang/comments/rem1/", Author: "[removed]", Subreddit: "golang", CreatedUTC: 1748649600.0, FullName: "t3_rem1"},
	}, nil)

	server := fakeHTTPServer(body)
	items, err := fetcher.NewRedditFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/golang",
	})
	if err != nil {
		t.Fatalf("fetch reddit: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items (removed skipped), got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// Error classification tests
// ---------------------------------------------------------------------------

func TestRedditFetcherClassifiesRateLimitAsRateLimitedError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Too Many Requests", "error": 429}`)),
			Request:    req,
		}, nil
	})}

	_, err := fetcher.NewRedditFetcher(client).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  "https://www.reddit.com/r/golang",
	})
	if err == nil {
		t.Fatal("expected rate limit error, got nil")
	}
	if !fetcher.IsRateLimited(err) {
		t.Fatalf("expected rate limited error, got: %v", err)
	}
}

func TestRedditFetcherClassifiesPrivateSubredditAsForbiddenError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Forbidden", "error": 403}`)),
			Request:    req,
		}, nil
	})}

	_, err := fetcher.NewRedditFetcher(client).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  "https://www.reddit.com/r/privatesub",
	})
	if err == nil {
		t.Fatal("expected forbidden error, got nil")
	}
	if !fetcher.IsForbidden(err) {
		t.Fatalf("expected forbidden error, got: %v", err)
	}
}

func TestRedditFetcherClassifiesNotFoundAsNotFoundError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Not Found", "error": 404}`)),
			Request:    req,
		}, nil
	})}

	_, err := fetcher.NewRedditFetcher(client).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  "https://www.reddit.com/r/nonexistent",
	})
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
	if !fetcher.IsNotFound(err) {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NSFW filtering
// ---------------------------------------------------------------------------

func TestRedditFetcherFiltersNSFWPostsByDefault(t *testing.T) {
	body := redditListingJSON([]redditTestPost{
		{Title: "Safe post", URL: "https://www.reddit.com/r/golang/comments/safe1/", Permalink: "/r/golang/comments/safe1/", Author: "gopher", Subreddit: "golang", CreatedUTC: 1748649600.0, Over18: false, FullName: "t3_safe1"},
		{Title: "NSFW post", URL: "https://www.reddit.com/r/golang/comments/nsfw1/", Permalink: "/r/golang/comments/nsfw1/", Author: "gopher", Subreddit: "golang", CreatedUTC: 1748649600.0, Over18: true, FullName: "t3_nsfw1"},
	}, nil)

	server := fakeHTTPServer(body)
	items, err := fetcher.NewRedditFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/golang",
	})
	if err != nil {
		t.Fatalf("fetch reddit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (NSFW filtered), got %d", len(items))
	}
	if items[0].ExternalID != "t3_safe1" {
		t.Fatalf("expected safe post, got %q", items[0].ExternalID)
	}
}

func TestRedditFetcherAllowsNSFWWhenConfigured(t *testing.T) {
	body := redditListingJSON([]redditTestPost{
		{Title: "Safe post", URL: "https://www.reddit.com/r/golang/comments/safe1/", Permalink: "/r/golang/comments/safe1/", Author: "gopher", Subreddit: "golang", CreatedUTC: 1748649600.0, Over18: false, FullName: "t3_safe1"},
		{Title: "NSFW post", URL: "https://www.reddit.com/r/golang/comments/nsfw1/", Permalink: "/r/golang/comments/nsfw1/", Author: "gopher", Subreddit: "golang", CreatedUTC: 1748649600.0, Over18: true, FullName: "t3_nsfw1"},
	}, nil)

	server := fakeHTTPServer(body)
	f := fetcher.NewRedditFetcher(server.Client(), func(cfg *fetcher.RedditConfig) {
		cfg.AllowNSFW = true
	})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/golang",
	})
	if err != nil {
		t.Fatalf("fetch reddit: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (NSFW allowed), got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// Comment truncation
// ---------------------------------------------------------------------------

func TestRedditFetcherTruncatesCommentSamplesToMax(t *testing.T) {
	var comments []redditTestComment
	for i := 0; i < 25; i++ {
		comments = append(comments, redditTestComment{
			Author:     fmt.Sprintf("user%d", i),
			Body:       fmt.Sprintf("Comment %d", i),
			CreatedUTC: 1748649600.0 + float64(i),
			FullName:   fmt.Sprintf("t1_cmt%d", i),
		})
	}
	body := redditListingJSON([]redditTestPost{{
		Title:      "Popular thread",
		URL:        "https://www.reddit.com/r/golang/comments/pop1/",
		Permalink:  "/r/golang/comments/pop1/",
		Author:     "gopher",
		Subreddit:  "golang",
		CreatedUTC: 1748649600.0,
		SelfText:   "Very popular.",
		Over18:     false,
		FullName:   "t3_pop1",
	}}, comments)

	server := fakeHTTPServer(body)
	items, err := fetcher.NewRedditFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/golang",
	})
	if err != nil {
		t.Fatalf("fetch reddit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if len(items[0].CommentSamples) > 10 {
		t.Fatalf("expected at most 10 comment samples, got %d", len(items[0].CommentSamples))
	}
}

// ---------------------------------------------------------------------------
// Source type validation
// ---------------------------------------------------------------------------

func TestRedditFetcherRejectsNonRedditSourceType(t *testing.T) {
	_, err := fetcher.NewRedditFetcher(nil).Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  "https://www.reddit.com/r/golang",
	})
	if err == nil {
		t.Fatal("expected error for non-reddit source type, got nil")
	}
}

// ---------------------------------------------------------------------------
// Empty listing
// ---------------------------------------------------------------------------

func TestRedditFetcherReturnsEmptyForEmptySubreddit(t *testing.T) {
	body := redditListingJSON(nil, nil)
	server := fakeHTTPServer(body)
	items, err := fetcher.NewRedditFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/emptysub",
	})
	if err != nil {
		t.Fatalf("fetch empty subreddit: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items for empty subreddit, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// URL deduplication
// ---------------------------------------------------------------------------

func TestRedditFetcherDeduplicatesByURL(t *testing.T) {
	// Two posts with the same external URL should be deduplicated
	body := redditListingJSON([]redditTestPost{
		{Title: "First post", URL: "https://example.com/article", Permalink: "/r/news/comments/aaa/first/", Author: "user1", Subreddit: "news", CreatedUTC: 1748649600.0, FullName: "t3_aaa", IsSelf: false},
		{Title: "Second post", URL: "https://example.com/article", Permalink: "/r/news/comments/bbb/second/", Author: "user2", Subreddit: "news", CreatedUTC: 1748649700.0, FullName: "t3_bbb", IsSelf: false},
	}, nil)

	server := fakeHTTPServer(body)
	items, err := fetcher.NewRedditFetcher(server.Client()).Fetch(context.Background(), fetcher.Source{
		ID:   "src_reddit",
		Type: fetcher.SourceTypeReddit,
		URL:  server.URL + "/r/news",
	})
	if err != nil {
		t.Fatalf("fetch reddit: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after URL dedup, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type redditTestPost struct {
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Permalink  string  `json:"permalink"`
	Author     string  `json:"author"`
	Subreddit  string  `json:"subreddit"`
	CreatedUTC float64 `json:"created_utc"`
	SelfText   string  `json:"selftext"`
	Over18     bool    `json:"over_18"`
	FullName   string  `json:"name"`
	IsSelf     bool    `json:"is_self"`
	Score      int     `json:"score"`
}

type redditTestComment struct {
	Author     string  `json:"author"`
	Body       string  `json:"body"`
	CreatedUTC float64 `json:"created_utc"`
	FullName   string  `json:"name"`
}

func redditListingJSON(posts []redditTestPost, comments []redditTestComment) string {
	var children []map[string]interface{}
	for _, p := range posts {
		children = append(children, map[string]interface{}{
			"kind": "t3",
			"data": map[string]interface{}{
				"title":        p.Title,
				"url":          p.URL,
				"permalink":    p.Permalink,
				"author":       p.Author,
				"subreddit":    p.Subreddit,
				"created_utc":  p.CreatedUTC,
				"selftext":     p.SelfText,
				"over_18":      p.Over18,
				"name":         p.FullName,
				"is_self":      p.IsSelf,
				"score":        p.Score,
				"num_comments": len(comments),
			},
		})
	}
	var commentChildren []map[string]interface{}
	for _, c := range comments {
		commentChildren = append(commentChildren, map[string]interface{}{
			"kind": "t1",
			"data": map[string]interface{}{
				"author":      c.Author,
				"body":        c.Body,
				"created_utc": c.CreatedUTC,
				"name":        c.FullName,
			},
		})
	}
	listing := map[string]interface{}{
		"data": map[string]interface{}{
			"children": children,
		},
	}
	commentListing := map[string]interface{}{
		"data": map[string]interface{}{
			"children": commentChildren,
		},
	}
	result := []interface{}{listing, commentListing}
	b, _ := json.Marshal(result)
	return string(b)
}

