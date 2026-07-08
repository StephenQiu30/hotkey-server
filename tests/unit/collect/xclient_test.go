package collect_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/service"
)

func TestParseTweet(t *testing.T) {
	raw := `{"data": {"id": "123", "text": "hello world", "author_id": "456"}, "includes": {"users": [{"id": "456", "name": "Test User", "username": "test"}]}}`
	tweet, err := service.ParseTweet([]byte(raw))
	if err != nil {
		t.Fatalf("ParseTweet failed: %v", err)
	}
	if tweet.ID != "123" {
		t.Errorf("expected ID 123, got %s", tweet.ID)
	}
	if tweet.Text != "hello world" {
		t.Errorf("expected text 'hello world', got %s", tweet.Text)
	}
	if tweet.AuthorID != "456" {
		t.Errorf("expected author 456, got %s", tweet.AuthorID)
	}
	if tweet.AuthorName != "Test User" {
		t.Errorf("expected 'Test User', got %s", tweet.AuthorName)
	}
	if tweet.AuthorHandle != "test" {
		t.Errorf("expected 'test', got %s", tweet.AuthorHandle)
	}
}

func TestParseTweetNoIncludes(t *testing.T) {
	raw := `{"data": {"id": "789", "text": "no includes"}}`
	tweet, err := service.ParseTweet([]byte(raw))
	if err != nil {
		t.Fatalf("ParseTweet failed: %v", err)
	}
	if tweet.ID != "789" {
		t.Errorf("expected ID 789, got %s", tweet.ID)
	}
	if tweet.AuthorName != "" {
		t.Errorf("expected empty author name for no includes, got %s", tweet.AuthorName)
	}
}

func TestParseTweetError(t *testing.T) {
	_, err := service.ParseTweet([]byte(`invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseTweetEmptyData(t *testing.T) {
	_, err := service.ParseTweet([]byte(`{"data": {}}`))
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}
