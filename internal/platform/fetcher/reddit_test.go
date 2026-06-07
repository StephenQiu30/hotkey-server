package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedditFetcherRejectsNonRedditSource(t *testing.T) {
	f := NewRedditFetcher(nil, RedditConfig{})
	_, err := f.Fetch(context.Background(), Source{Type: SourceTypeRSS, URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for non-reddit source")
	}
}

func TestRedditFetcherFetchesSubredditPosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := redditListing{
			Data: redditListingData{
				Children: []redditChild{
					{Data: redditPost{
						ID:          "abc123",
						Title:       "Test Post",
						URL:         "https://example.com/article",
						Author:      "testuser",
						Score:       42,
						NumComments: 10,
						CreatedUTC:  1700000000,
						Subreddit:   "golang",
						SelfText:    "",
					}},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewRedditFetcher(srv.Client(), RedditConfig{CommentSampleLimit: 3})
	items, err := f.Fetch(context.Background(), Source{
		Type: SourceTypeReddit,
		URL:  srv.URL + "/r/golang/hot.json",
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Test Post" {
		t.Errorf("title = %q, want %q", items[0].Title, "Test Post")
	}
	if items[0].ExternalID != "abc123" {
		t.Errorf("external_id = %q, want %q", items[0].ExternalID, "abc123")
	}
	if items[0].Score != 42 {
		t.Errorf("score = %d, want 42", items[0].Score)
	}
	if items[0].Descendants != 10 {
		t.Errorf("descendants = %d, want 10", items[0].Descendants)
	}
}

func TestRedditFetcherSkipsDeletedPosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := redditListing{
			Data: redditListingData{
				Children: []redditChild{
					{Data: redditPost{ID: "del1", Title: "[deleted]", Author: "[deleted]", Score: 5}},
					{Data: redditPost{ID: "rem1", Title: "Removed Post", Author: "[removed]", Score: 3}},
					{Data: redditPost{ID: "ok1", Title: "Good Post", Author: "user1", Score: 10, URL: "https://example.com"}},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewRedditFetcher(srv.Client(), RedditConfig{})
	items, err := f.Fetch(context.Background(), Source{
		Type: SourceTypeReddit,
		URL:  srv.URL + "/r/test/hot.json",
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (deleted/removed skipped), got %d", len(items))
	}
	if items[0].Title != "Good Post" {
		t.Errorf("title = %q, want %q", items[0].Title, "Good Post")
	}
}

func TestRedditFetcherHandlesRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	f := NewRedditFetcher(srv.Client(), RedditConfig{})
	_, err := f.Fetch(context.Background(), Source{
		Type: SourceTypeReddit,
		URL:  srv.URL + "/r/test/hot.json",
	})
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	var re *RedditError
	if !errors.As(err, &re) || re.Class != "rate_limit" {
		t.Errorf("expected rate_limit RedditError, got: %v", err)
	}
}

func TestRedditFetcherHandlesForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	f := NewRedditFetcher(srv.Client(), RedditConfig{})
	_, err := f.Fetch(context.Background(), Source{
		Type: SourceTypeReddit,
		URL:  srv.URL + "/r/private/hot.json",
	})
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	var re *RedditError
	if !errors.As(err, &re) || re.Class != "auth" {
		t.Errorf("expected auth RedditError, got: %v", err)
	}
}

func TestRedditFetcherCommentSamples(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Post listing
		if r.URL.Path == "/r/test/hot.json" {
			resp := redditListing{
				Data: redditListingData{
					Children: []redditChild{
						{Data: redditPost{
							ID:          "post1",
							Title:       "Post with comments",
							URL:         "https://example.com",
							Score:       100,
							NumComments: 3,
							CreatedUTC:  1700000000,
						}},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Comment endpoint
		resp := []json.RawMessage{
			nil, // post data (ignored)
			json.RawMessage(`{"data":{"children":[{"data":{"id":"c1","body":"Great article!","author":"user1","score":5,"created_utc":1700000100}}]}}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewRedditFetcher(srv.Client(), RedditConfig{CommentSampleLimit: 3})
	items, err := f.Fetch(context.Background(), Source{
		Type: SourceTypeReddit,
		URL:  srv.URL + "/r/test/hot.json",
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if len(items[0].CommentSamples) != 1 {
		t.Fatalf("expected 1 comment sample, got %d", len(items[0].CommentSamples))
	}
	if items[0].CommentSamples[0].Text != "Great article!" {
		t.Errorf("comment text = %q, want %q", items[0].CommentSamples[0].Text, "Great article!")
	}
}

func TestRedditFetcherDeduplicatesByURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := redditListing{
			Data: redditListingData{
				Children: []redditChild{
					{Data: redditPost{ID: "p1", Title: "First", URL: "https://example.com/same", Author: "u1", Score: 10}},
					{Data: redditPost{ID: "p2", Title: "Second", URL: "https://example.com/same", Author: "u2", Score: 20}},
					{Data: redditPost{ID: "p3", Title: "Different", URL: "https://example.com/other", Author: "u3", Score: 5}},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewRedditFetcher(srv.Client(), RedditConfig{})
	items, err := f.Fetch(context.Background(), Source{
		Type: SourceTypeReddit,
		URL:  srv.URL + "/r/test/hot.json",
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (deduped), got %d", len(items))
	}
}

func TestRedditFetcherSkipsNSFW(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := redditListing{
			Data: redditListingData{
				Children: []redditChild{
					{Data: redditPost{ID: "nsfw1", Title: "NSFW Post", URL: "https://example.com/nsfw", Author: "u1", Score: 100, Over18: true}},
					{Data: redditPost{ID: "safe1", Title: "Safe Post", URL: "https://example.com/safe", Author: "u2", Score: 50, Over18: false}},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewRedditFetcher(srv.Client(), RedditConfig{AllowNSFW: false})
	items, err := f.Fetch(context.Background(), Source{
		Type: SourceTypeReddit,
		URL:  srv.URL + "/r/test/hot.json",
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (NSFW filtered), got %d", len(items))
	}
	if items[0].Title != "Safe Post" {
		t.Errorf("title = %q, want %q", items[0].Title, "Safe Post")
	}
}
