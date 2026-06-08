package fetcher_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
)

func TestHNFetcherParsesTopStoriesFromFakeAPI(t *testing.T) {
	server := fakeHNAPI(t, map[string]any{
		"/v0/topstories.json": []int64{1001, 1002},
		"/v0/item/1001.json":  hnStory(1001, "Go 2.0 Released", "https://go.dev/blog/go2", "gopher", 500, 120),
		"/v0/item/1002.json":  hnStory(1002, "AI Agents Explained", "https://example.com/ai", "researcher", 300, 80),
	})

	f := fetcher.NewHNFetcher(server.Client(), fetcher.HNConfig{CommentSampleLimit: 5})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_hn",
		Type: fetcher.SourceTypeHackerNews,
		URL:  server.URL + "/v0/topstories.json",
	})
	if err != nil {
		t.Fatalf("fetch hn: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}

	item := items[0]
	if item.Title != "Go 2.0 Released" {
		t.Errorf("title = %q, want %q", item.Title, "Go 2.0 Released")
	}
	if item.URL != "https://go.dev/blog/go2" {
		t.Errorf("url = %q, want %q", item.URL, "https://go.dev/blog/go2")
	}
	if item.ExternalID != "1001" {
		t.Errorf("externalID = %q, want %q", item.ExternalID, "1001")
	}
	if item.Score != 500 {
		t.Errorf("score = %d, want 500", item.Score)
	}
	if item.Descendants != 120 {
		t.Errorf("descendants = %d, want 120", item.Descendants)
	}
}

func TestHNFetcherDeduplicatesByURL(t *testing.T) {
	server := fakeHNAPI(t, map[string]any{
		"/v0/topstories.json": []int64{2001, 2002},
		"/v0/item/2001.json":  hnStory(2001, "Same Article", "https://example.com/same", "user1", 100, 10),
		"/v0/item/2002.json":  hnStory(2002, "Same Article RePost", "https://example.com/same", "user2", 50, 5),
	})

	f := fetcher.NewHNFetcher(server.Client(), fetcher.HNConfig{CommentSampleLimit: 5})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_hn",
		Type: fetcher.SourceTypeHackerNews,
		URL:  server.URL + "/v0/topstories.json",
	})
	if err != nil {
		t.Fatalf("fetch hn: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 deduplicated item, got %d: %+v", len(items), items)
	}
}

func TestHNFetcherSkipsDeletedItems(t *testing.T) {
	server := fakeHNAPI(t, map[string]any{
		"/v0/topstories.json": []int64{3001, 3002},
		"/v0/item/3001.json":  hnStory(3001, "Valid Post", "https://example.com/valid", "author", 100, 10),
		"/v0/item/3002.json":  map[string]any{"id": 3002, "deleted": true},
	})

	f := fetcher.NewHNFetcher(server.Client(), fetcher.HNConfig{CommentSampleLimit: 5})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_hn",
		Type: fetcher.SourceTypeHackerNews,
		URL:  server.URL + "/v0/topstories.json",
	})
	if err != nil {
		t.Fatalf("fetch hn: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (deleted filtered), got %d: %+v", len(items), items)
	}
	if items[0].Title != "Valid Post" {
		t.Errorf("title = %q, want %q", items[0].Title, "Valid Post")
	}
}

func TestHNFetcherSkipsDeadItems(t *testing.T) {
	server := fakeHNAPI(t, map[string]any{
		"/v0/topstories.json": []int64{4001},
		"/v0/item/4001.json":  map[string]any{"id": 4001, "dead": true, "title": "Dead Post", "url": "https://example.com/dead"},
	})

	f := fetcher.NewHNFetcher(server.Client(), fetcher.HNConfig{CommentSampleLimit: 5})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_hn",
		Type: fetcher.SourceTypeHackerNews,
		URL:  server.URL + "/v0/topstories.json",
	})
	if err != nil {
		t.Fatalf("fetch hn: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items (dead filtered), got %d: %+v", len(items), items)
	}
}

func TestHNFetcherReturnsEmptyListForEmptyStories(t *testing.T) {
	server := fakeHNAPI(t, map[string]any{
		"/v0/topstories.json": []int64{},
	})

	f := fetcher.NewHNFetcher(server.Client(), fetcher.HNConfig{CommentSampleLimit: 5})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_hn",
		Type: fetcher.SourceTypeHackerNews,
		URL:  server.URL + "/v0/topstories.json",
	})
	if err != nil {
		t.Fatalf("fetch hn empty: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items for empty stories, got %d", len(items))
	}
}

func TestHNFetcherCommentSampleRespectsLimit(t *testing.T) {
	comments := []int64{5001, 5002, 5003, 5004, 5005, 5006, 5007, 5008}
	itemData := hnStory(6001, "Popular Post", "https://example.com/popular", "author", 1000, len(comments))
	itemData["kids"] = comments

	commentItems := map[string]any{}
	commentItems["/v0/item/6001.json"] = itemData
	for i, cid := range comments {
		commentItems[fmt.Sprintf("/v0/item/%d.json", cid)] = hnComment(cid, fmt.Sprintf("Comment %d", i+1), "commenter")
	}

	server := fakeHNAPI(t, map[string]any{
		"/v0/topstories.json": []int64{6001},
		"/v0/item/6001.json":  itemData,
		"/v0/item/5001.json":  hnComment(5001, "Comment 1", "commenter"),
		"/v0/item/5002.json":  hnComment(5002, "Comment 2", "commenter"),
		"/v0/item/5003.json":  hnComment(5003, "Comment 3", "commenter"),
		"/v0/item/5004.json":  hnComment(5004, "Comment 4", "commenter"),
		"/v0/item/5005.json":  hnComment(5005, "Comment 5", "commenter"),
		"/v0/item/5006.json":  hnComment(5006, "Comment 6", "commenter"),
		"/v0/item/5007.json":  hnComment(5007, "Comment 7", "commenter"),
		"/v0/item/5008.json":  hnComment(5008, "Comment 8", "commenter"),
	})

	f := fetcher.NewHNFetcher(server.Client(), fetcher.HNConfig{CommentSampleLimit: 3})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_hn",
		Type: fetcher.SourceTypeHackerNews,
		URL:  server.URL + "/v0/topstories.json",
	})
	if err != nil {
		t.Fatalf("fetch hn: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if len(items[0].CommentSamples) != 3 {
		t.Errorf("comment samples = %d, want 3 (limit)", len(items[0].CommentSamples))
	}
}

func TestHNFetcherDefaultsCommentLimitTo5(t *testing.T) {
	server := fakeHNAPI(t, map[string]any{
		"/v0/topstories.json": []int64{7001},
		"/v0/item/7001.json":  hnStory(7001, "Post", "https://example.com/post", "author", 100, 10),
	})

	f := fetcher.NewHNFetcher(server.Client(), fetcher.HNConfig{})
	if f.CommentSampleLimit() != 5 {
		t.Errorf("default comment limit = %d, want 5", f.CommentSampleLimit())
	}
}

func TestHNFetcherRejectsInvalidSourceType(t *testing.T) {
	f := fetcher.NewHNFetcher(nil, fetcher.HNConfig{})
	_, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_rss",
		Type: fetcher.SourceTypeRSS,
		URL:  "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for non-hackernews source type")
	}
}

func TestHNFetcherHandlesItemFetchError(t *testing.T) {
	var calls atomic.Int32
	server := &httpServer{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.URL.Path == "/v0/topstories.json" {
				json.NewEncoder(w).Encode([]int64{8001, 8002})
				return
			}
			if r.URL.Path == "/v0/item/8001.json" {
				json.NewEncoder(w).Encode(hnStory(8001, "Good", "https://example.com/good", "u", 10, 1))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}),
	}
	srv := server.start(t)
	defer srv.Close()

	f := fetcher.NewHNFetcher(srv.Client(), fetcher.HNConfig{CommentSampleLimit: 5})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_hn",
		Type: fetcher.SourceTypeHackerNews,
		URL:  srv.URL + "/v0/topstories.json",
	})
	if err != nil {
		t.Fatalf("fetch hn should not fail on single item error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (other skipped), got %d", len(items))
	}
}

func TestHNFetcherNewStoriesEndpoint(t *testing.T) {
	server := fakeHNAPI(t, map[string]any{
		"/v0/newstories.json": []int64{9001},
		"/v0/item/9001.json":  hnStory(9001, "New Story", "https://example.com/new", "newuser", 5, 0),
	})

	f := fetcher.NewHNFetcher(server.Client(), fetcher.HNConfig{CommentSampleLimit: 5})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_hn",
		Type: fetcher.SourceTypeHackerNews,
		URL:  server.URL + "/v0/newstories.json",
	})
	if err != nil {
		t.Fatalf("fetch hn new: %v", err)
	}
	if len(items) != 1 || items[0].Title != "New Story" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestHNFetcherBestStoriesEndpoint(t *testing.T) {
	server := fakeHNAPI(t, map[string]any{
		"/v0/beststories.json": []int64{9501},
		"/v0/item/9501.json":   hnStory(9501, "Best Story", "https://example.com/best", "bestuser", 2000, 500),
	})

	f := fetcher.NewHNFetcher(server.Client(), fetcher.HNConfig{CommentSampleLimit: 5})
	items, err := f.Fetch(context.Background(), fetcher.Source{
		ID:   "src_hn",
		Type: fetcher.SourceTypeHackerNews,
		URL:  server.URL + "/v0/beststories.json",
	})
	if err != nil {
		t.Fatalf("fetch hn best: %v", err)
	}
	if len(items) != 1 || items[0].Score != 2000 {
		t.Fatalf("unexpected items: %+v", items)
	}
}

// --- helpers ---

func hnStory(id int64, title, url, author string, score, descendants int) map[string]any {
	return map[string]any{
		"id":          id,
		"title":       title,
		"url":         url,
		"by":          author,
		"score":       score,
		"descendants": descendants,
		"type":        "story",
		"time":        1717200000,
	}
}

func hnComment(id int64, text, author string) map[string]any {
	return map[string]any{
		"id":   id,
		"text": text,
		"by":   author,
		"type": "comment",
		"time": 1717200000,
	}
}

type httpServer struct {
	handler http.Handler
}

func (s *httpServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(s.handler)
	return srv
}

func fakeHNAPI(t *testing.T, routes map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, data := range routes {
		pathCopy := path
		dataCopy := data
		mux.HandleFunc(pathCopy, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(dataCopy)
		})
	}
	return httptest.NewServer(mux)
}
