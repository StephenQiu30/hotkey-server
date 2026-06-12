package content

import (
	"testing"
)

func TestNormalizeCreatesStableHash(t *testing.T) {
	post := RawPost{
		ID:           "123",
		AuthorID:     "author-1",
		AuthorName:   "OpenAI",
		AuthorHandle: "openai",
		Text:         " OpenAI Agent launch ",
		Language:     "en",
		LikeCount:    100,
	}
	normalized := Normalize(post, "x")
	if normalized.NormalizedHash == "" {
		t.Fatal("expected normalized hash")
	}
	// Same input should produce same hash
	normalized2 := Normalize(post, "x")
	if normalized.NormalizedHash != normalized2.NormalizedHash {
		t.Fatalf("expected stable hash, got %q and %q", normalized.NormalizedHash, normalized2.NormalizedHash)
	}
}

func TestNormalizeTrimsText(t *testing.T) {
	post := RawPost{
		ID:   "456",
		Text: "  hello world  ",
	}
	normalized := Normalize(post, "x")
	if normalized.ContentText != "hello world" {
		t.Errorf("expected trimmed text 'hello world', got %q", normalized.ContentText)
	}
}

func TestNormalizeGeneratesPostURL(t *testing.T) {
	post := RawPost{
		ID:           "789",
		AuthorHandle: "testuser",
	}
	normalized := Normalize(post, "x")
	expected := "https://x.com/testuser/status/789"
	if normalized.PostURL != expected {
		t.Errorf("expected post URL %q, got %q", expected, normalized.PostURL)
	}
}

func TestNormalizeDeduplicatesIdenticalContent(t *testing.T) {
	post1 := RawPost{ID: "a1", Text: "same content", AuthorHandle: "user1"}
	post2 := RawPost{ID: "a2", Text: "same content", AuthorHandle: "user1"}
	n1 := Normalize(post1, "x")
	n2 := Normalize(post2, "x")
	if n1.NormalizedHash != n2.NormalizedHash {
		t.Error("expected identical content to produce same hash")
	}
}

func TestNormalizeDifferentiatesDifferentContent(t *testing.T) {
	post1 := RawPost{ID: "b1", Text: "content A", AuthorHandle: "user1"}
	post2 := RawPost{ID: "b2", Text: "content B", AuthorHandle: "user1"}
	n1 := Normalize(post1, "x")
	n2 := Normalize(post2, "x")
	if n1.NormalizedHash == n2.NormalizedHash {
		t.Error("expected different content to produce different hash")
	}
}
