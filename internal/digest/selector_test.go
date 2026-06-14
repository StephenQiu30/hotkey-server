package digest

import (
	"context"
	"testing"
	"time"
)

// fakeTopicFilter is an in-memory implementation of TopicFilter for testing.
type fakeTopicFilter struct {
	topics []TopicEntry
	posts  map[int64][]PostEntry // topicID -> posts
	err    error
}

func (f *fakeTopicFilter) ListTopicsForDay(_ context.Context, _ int64, _ Window) ([]TopicEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.topics, nil
}

func (f *fakeTopicFilter) FetchRepresentativePosts(_ context.Context, topicID int64, limit int) ([]PostEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	posts := f.posts[topicID]
	if len(posts) > limit {
		posts = posts[:limit]
	}
	return posts, nil
}

func TestSelectTopicsForDay_TopN(t *testing.T) {
	topics := []TopicEntry{
		{ID: 1, Title: "hot topic", Heat: 100.0},
		{ID: 2, Title: "warm topic", Heat: 50.0},
		{ID: 3, Title: "cool topic", Heat: 10.0},
		{ID: 4, Title: "cold topic", Heat: 5.0},
		{ID: 5, Title: "frozen topic", Heat: 1.0},
	}

	filter := &fakeTopicFilter{topics: topics}
	svc := NewService(filter)

	ctx := context.Background()
	cst := CST
	date, _ := time.ParseInLocation("2006-01-02", "2026-06-14", cst)
	result, err := svc.SelectTopicsForDay(ctx, 1, date, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("got %d topics, want 3", len(result))
	}
	// Should be sorted by heat DESC
	if result[0].Heat != 100.0 {
		t.Errorf("first topic heat = %f, want 100", result[0].Heat)
	}
	if result[1].Heat != 50.0 {
		t.Errorf("second topic heat = %f, want 50", result[1].Heat)
	}
	if result[2].Heat != 10.0 {
		t.Errorf("third topic heat = %f, want 10", result[2].Heat)
	}
}

func TestSelectTopicsForDay_ActiveOnly(t *testing.T) {
	// The DB query already filters status='active', so the filter interface
	// only returns active topics. Verify that an empty result is handled.
	filter := &fakeTopicFilter{topics: nil}
	svc := NewService(filter)

	ctx := context.Background()
	cst := CST
	date, _ := time.ParseInLocation("2006-01-02", "2026-06-14", cst)
	result, err := svc.SelectTopicsForDay(ctx, 1, date, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %d topics, want 0 for empty filter result", len(result))
	}
}

func TestSelectTopicsForDay_FewerThanN(t *testing.T) {
	topics := []TopicEntry{
		{ID: 1, Title: "only topic", Heat: 42.0},
	}

	filter := &fakeTopicFilter{topics: topics}
	svc := NewService(filter)

	ctx := context.Background()
	cst := CST
	date, _ := time.ParseInLocation("2006-01-02", "2026-06-14", cst)
	result, err := svc.SelectTopicsForDay(ctx, 1, date, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("got %d topics, want 1", len(result))
	}
}

func TestSelectRepresentativePosts_Top3(t *testing.T) {
	posts := map[int64][]PostEntry{
		1: {
			{PostID: 101, AuthorName: "alice", ContentExcerpt: "hello world", PostURL: "https://x.com/101", MembershipScore: 0.9},
			{PostID: 102, AuthorName: "bob", ContentExcerpt: "foo bar", PostURL: "https://x.com/102", MembershipScore: 0.7},
			{PostID: 103, AuthorName: "carol", ContentExcerpt: "baz qux", PostURL: "https://x.com/103", MembershipScore: 0.5},
			{PostID: 104, AuthorName: "dave", ContentExcerpt: "extra", PostURL: "https://x.com/104", MembershipScore: 0.3},
		},
	}

	filter := &fakeTopicFilter{posts: posts}
	svc := NewService(filter)

	ctx := context.Background()
	result, err := svc.SelectRepresentativePosts(ctx, 1, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d posts, want 3", len(result))
	}
	// Verify fields are populated
	for _, p := range result {
		if p.AuthorName == "" {
			t.Error("author_name should not be empty")
		}
		if p.ContentExcerpt == "" {
			t.Error("content_excerpt should not be empty")
		}
		if p.PostURL == "" {
			t.Error("post_url should not be empty")
		}
	}
}

func TestSelectRepresentativePosts_Empty(t *testing.T) {
	filter := &fakeTopicFilter{posts: map[int64][]PostEntry{}}
	svc := NewService(filter)

	ctx := context.Background()
	result, err := svc.SelectRepresentativePosts(ctx, 999, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %d posts, want 0", len(result))
	}
}

func TestBuildDayDigest_Orchestration(t *testing.T) {
	topics := []TopicEntry{
		{ID: 1, Title: "AI news", Heat: 80.0},
		{ID: 2, Title: "crypto", Heat: 60.0},
	}
	posts := map[int64][]PostEntry{
		1: {
			{PostID: 101, AuthorName: "alice", ContentExcerpt: "AI breakthrough", PostURL: "https://x.com/101", MembershipScore: 0.9},
			{PostID: 102, AuthorName: "bob", ContentExcerpt: "LLM update", PostURL: "https://x.com/102", MembershipScore: 0.8},
		},
		2: {
			{PostID: 201, AuthorName: "carol", ContentExcerpt: "BTC rally", PostURL: "https://x.com/201", MembershipScore: 0.95},
		},
	}

	filter := &fakeTopicFilter{topics: topics, posts: posts}
	svc := NewService(filter)

	ctx := context.Background()
	cst := CST
	now, _ := time.ParseInLocation("2006-01-02 15:04", "2026-06-14 10:00", cst)

	digest, err := svc.BuildDayDigest(ctx, 1, now, "today", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(digest.Topics) != 2 {
		t.Fatalf("got %d topics, want 2", len(digest.Topics))
	}
	if digest.Topics[0].Topic.Title != "AI news" {
		t.Errorf("first topic = %q, want %q", digest.Topics[0].Topic.Title, "AI news")
	}
	if len(digest.Topics[0].Posts) != 2 {
		t.Errorf("first topic posts = %d, want 2", len(digest.Topics[0].Posts))
	}
	if len(digest.Topics[1].Posts) != 1 {
		t.Errorf("second topic posts = %d, want 1", len(digest.Topics[1].Posts))
	}
	// Verify export date is today CST midnight
	wantDate, _ := time.ParseInLocation("2006-01-02", "2026-06-14", cst)
	if !digest.ExportDate.Equal(wantDate) {
		t.Errorf("export date = %v, want %v", digest.ExportDate, wantDate)
	}
}

func TestBuildDayDigest_DefaultTopN(t *testing.T) {
	filter := &fakeTopicFilter{topics: nil, posts: map[int64][]PostEntry{}}
	svc := NewService(filter)

	ctx := context.Background()
	cst := CST
	now, _ := time.ParseInLocation("2006-01-02 15:04", "2026-06-14 10:00", cst)

	// topN=0 should default to DefaultTopN
	digest, err := svc.BuildDayDigest(ctx, 1, now, "yesterday", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest == nil {
		t.Fatal("digest should not be nil")
	}
	// Verify export date is yesterday
	wantDate, _ := time.ParseInLocation("2006-01-02", "2026-06-13", cst)
	if !digest.ExportDate.Equal(wantDate) {
		t.Errorf("export date = %v, want %v", digest.ExportDate, wantDate)
	}
}
