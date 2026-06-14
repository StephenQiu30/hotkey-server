package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/digest"
	"github.com/StephenQiu30/hotkey-server/internal/llm"
)

// --- Mock TopicFilter ---

type mockTopicFilter struct {
	topics []digest.TopicEntry
	posts  map[int64][]digest.PostEntry
}

func (m *mockTopicFilter) ListTopicsForDay(_ context.Context, _ int64, _ digest.Window) ([]digest.TopicEntry, error) {
	return m.topics, nil
}

func (m *mockTopicFilter) FetchRepresentativePosts(_ context.Context, topicID int64, _ int) ([]digest.PostEntry, error) {
	if posts, ok := m.posts[topicID]; ok {
		return posts, nil
	}
	return nil, nil
}

// --- Mocks ---

type mockTopicExporter struct {
	exported map[string]bool
	failures map[string]string
}

func newMockTopicExporter() *mockTopicExporter {
	return &mockTopicExporter{
		exported: make(map[string]bool),
		failures: make(map[string]string),
	}
}

func (m *mockTopicExporter) key(topicID int64, date string) string {
	return string(rune(topicID)) + ":" + date
}

func (m *mockTopicExporter) IsExported(_ context.Context, topicID int64, date string) (bool, error) {
	return m.exported[m.key(topicID, date)], nil
}

func (m *mockTopicExporter) MarkExported(_ context.Context, topicID int64, date string) error {
	m.exported[m.key(topicID, date)] = true
	return nil
}

func (m *mockTopicExporter) MarkFailed(_ context.Context, topicID int64, date string, reason string) error {
	m.failures[m.key(topicID, date)] = reason
	return nil
}

type mockVaultWriter struct {
	written map[string]string
	err     error
}

func newMockVaultWriter() *mockVaultWriter {
	return &mockVaultWriter{
		written: make(map[string]string),
	}
}

func (m *mockVaultWriter) WriteAtomic(path, content string) error {
	if m.err != nil {
		return m.err
	}
	m.written[path] = content
	return nil
}

type mockLLMClientForPublish struct {
	summary string
	err     error
}

func (m *mockLLMClientForPublish) SummarizeTopic(_ context.Context, _ llm.TopicSummaryInput) (string, error) {
	return m.summary, m.err
}

type failingLLMClientForPublish struct {
	failOnTopic int64
	callCount   int
}

func (m *failingLLMClientForPublish) SummarizeTopic(_ context.Context, in llm.TopicSummaryInput) (string, error) {
	m.callCount++
	// Fail on first call
	if m.callCount == 1 {
		return "", errors.New("llm timeout")
	}
	return "summary", nil
}

// --- Tests ---

func TestPublishDailyTopics_PublishesTopics(t *testing.T) {
	// 09:00 CST — after gate, first run today
	now := time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC)

	// Create a mock digest service
	filter := &mockTopicFilter{
		topics: []digest.TopicEntry{
			{ID: 1, Title: "AI 监管政策", Heat: 85.5},
		},
		posts: map[int64][]digest.PostEntry{
			1: {
				{PostID: 1, AuthorName: "user1", ContentExcerpt: "post text", PostURL: "https://x.com/1"},
			},
		},
	}
	digestSvc := digest.NewService(filter)

	exporter := newMockTopicExporter()
	writer := newMockVaultWriter()
	llmClient := &mockLLMClientForPublish{summary: "AI 监管摘要"}

	job := NewPublishDailyTopicsJob(
		digestSvc,
		llmClient,
		exporter,
		writer,
		"/tmp/vault",
		MonitorConfig{ID: 1, Name: "AI Monitor", Slug: "ai"},
	)

	results, err := job.Run(context.Background(), now, "yesterday")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 1 result (published)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "published" {
		t.Fatalf("expected status=published, got %s", results[0].Status)
	}
}

func TestPublishDailyTopics_LLMFailureDoesNotBlockOtherTopics(t *testing.T) {
	now := time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC)

	// Create a mock digest service with 2 topics
	filter := &mockTopicFilter{
		topics: []digest.TopicEntry{
			{ID: 1, Title: "Topic A", Heat: 90},
			{ID: 2, Title: "Topic B", Heat: 80},
		},
		posts: map[int64][]digest.PostEntry{
			1: {{PostID: 1, AuthorName: "user1", ContentExcerpt: "post1", PostURL: "https://x.com/1"}},
			2: {{PostID: 2, AuthorName: "user2", ContentExcerpt: "post2", PostURL: "https://x.com/2"}},
		},
	}
	digestSvc := digest.NewService(filter)

	exporter := newMockTopicExporter()
	writer := newMockVaultWriter()
	llmClient := &failingLLMClientForPublish{failOnTopic: 1}

	job := NewPublishDailyTopicsJob(
		digestSvc,
		llmClient,
		exporter,
		writer,
		"/tmp/vault",
		MonitorConfig{ID: 1, Name: "AI Monitor", Slug: "ai"},
	)

	results, err := job.Run(context.Background(), now, "yesterday")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 results: first failed, second published
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != "failed" {
		t.Fatalf("first result: expected status=failed, got %s", results[0].Status)
	}
	if results[1].Status != "published" {
		t.Fatalf("second result: expected status=published, got %s", results[1].Status)
	}
}

func TestPublishDailyTopics_IdempotentPublish(t *testing.T) {
	now := time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC)

	// Create a mock digest service
	filter := &mockTopicFilter{
		topics: []digest.TopicEntry{
			{ID: 1, Title: "Topic A", Heat: 90},
		},
		posts: map[int64][]digest.PostEntry{
			1: {{PostID: 1, AuthorName: "user1", ContentExcerpt: "post1", PostURL: "https://x.com/1"}},
		},
	}
	digestSvc := digest.NewService(filter)

	exporter := newMockTopicExporter()
	writer := newMockVaultWriter()
	llmClient := &mockLLMClientForPublish{summary: "summary"}

	job := NewPublishDailyTopicsJob(
		digestSvc,
		llmClient,
		exporter,
		writer,
		"/tmp/vault",
		MonitorConfig{ID: 1, Name: "AI Monitor", Slug: "ai"},
	)

	// First run
	results1, err := job.Run(context.Background(), now, "yesterday")
	if err != nil {
		t.Fatalf("first run: unexpected error: %v", err)
	}

	// Second run — should be idempotent
	results2, err := job.Run(context.Background(), now, "yesterday")
	if err != nil {
		t.Fatalf("second run: unexpected error: %v", err)
	}

	// Both runs should return results
	if len(results1) != 1 || len(results2) != 1 {
		t.Fatalf("expected 1 result from each run, got %d and %d", len(results1), len(results2))
	}
}

func TestResolveExportDate_Yesterday(t *testing.T) {
	// 2026-06-14 12:00 CST = 2026-06-14 04:00 UTC
	now := time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)
	d := digest.ResolveExportDate(now, "yesterday")
	want := time.Date(2026, 6, 13, 0, 0, 0, 0, d.Location())
	if d != want {
		t.Fatalf("got %v, want %v", d, want)
	}
}

func TestResolveExportDate_Today(t *testing.T) {
	now := time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)
	d := digest.ResolveExportDate(now, "today")
	want := time.Date(2026, 6, 14, 0, 0, 0, 0, d.Location())
	if d != want {
		t.Fatalf("got %v, want %v", d, want)
	}
}
