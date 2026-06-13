package x_test

import (
	"os"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/x"
)

func TestSearchPostsParsesFixtures(t *testing.T) {
	data, err := os.ReadFile("../../../fixtures/platform/x/search_success.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	client := x.NewClient("test-token", "https://api.x.test")
	posts, meta, err := client.ParseSearchResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}
	if posts[0].ID != "1234567890" {
		t.Errorf("expected first post ID 1234567890, got %q", posts[0].ID)
	}
	if posts[0].AuthorHandle != "openai" {
		t.Errorf("expected first author handle openai, got %q", posts[0].AuthorHandle)
	}
	if posts[0].LikeCount != 1500 {
		t.Errorf("expected first post like count 1500, got %d", posts[0].LikeCount)
	}
	if meta.NextCursor != "NEXT123" {
		t.Fatalf("expected next cursor NEXT123, got %q", meta.NextCursor)
	}
	if meta.ResultCount != 2 {
		t.Errorf("expected result count 2, got %d", meta.ResultCount)
	}
}

func TestSearchPostsHandlesEmptyData(t *testing.T) {
	client := x.NewClient("test-token", "https://api.x.test")
	posts, meta, err := client.ParseSearchResponse([]byte(`{"data":[],"meta":{"result_count":0}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(posts))
	}
	if meta.ResultCount != 0 {
		t.Errorf("expected result count 0, got %d", meta.ResultCount)
	}
}
