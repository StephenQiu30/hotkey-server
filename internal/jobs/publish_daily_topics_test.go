package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- Mocks ---

type mockMonitorLister struct {
	ids []int64
	err error
}

func (m *mockMonitorLister) ListActiveIDs(_ context.Context) ([]int64, error) {
	return m.ids, m.err
}

type mockDigestService struct {
	topics []TopicCandidate
	err    error
}

func (m *mockDigestService) ListTopicsForDay(_ context.Context, _ int64, _ time.Time, _ int) ([]TopicCandidate, error) {
	return m.topics, m.err
}

type mockLLMClient struct {
	summary string
	err     error
}

func (m *mockLLMClient) SummarizeTopic(_ context.Context, _ TopicSummaryInput) (string, error) {
	return m.summary, m.err
}

type mockObsidianWriter struct {
	path string
	err  error
}

func (m *mockObsidianWriter) WriteTopicNote(_ context.Context, _ ObsidianNoteInput) (string, error) {
	return m.path, m.err
}

type mockExportRepo struct {
	lastRunDate string
	getErr      error
	setErr      error
	upsertErr   error
	upsertCalls []ExportRecord
}

func (m *mockExportRepo) GetLastRunDate(_ context.Context) (string, error) {
	return m.lastRunDate, m.getErr
}

func (m *mockExportRepo) SetLastRunDate(_ context.Context, _ string) error {
	return m.setErr
}

func (m *mockExportRepo) UpsertExport(_ context.Context, rec ExportRecord) (int64, error) {
	m.upsertCalls = append(m.upsertCalls, rec)
	if m.upsertErr != nil {
		return 0, m.upsertErr
	}
	return int64(len(m.upsertCalls)), nil
}

// --- Tests ---

func TestPublishDailyTopics_SkipsWhenNotTime(t *testing.T) {
	// 07:30 CST — before 08:00 gate
	now := time.Date(2026, 6, 13, 23, 30, 0, 0, time.UTC)
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")

	exports := &mockExportRepo{lastRunDate: ""}
	monitors := &mockMonitorLister{ids: []int64{1}}

	job := NewPublishDailyTopicsJob(
		monitors,
		&mockDigestService{},
		&mockLLMClient{},
		&mockObsidianWriter{},
		exports,
		sched,
		PublishDailyTopicsConfig{VaultPath: "/tmp/vault", Target: "yesterday", TopN: 20},
	)

	err := job.Run(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exports.upsertCalls) != 0 {
		t.Fatalf("expected no upsert calls, got %d", len(exports.upsertCalls))
	}
}

func TestPublishDailyTopics_SkipsWhenAlreadyRun(t *testing.T) {
	// 10:00 CST — after gate, but already ran today
	now := time.Date(2026, 6, 14, 2, 0, 0, 0, time.UTC)
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")

	exports := &mockExportRepo{lastRunDate: "2026-06-14"}
	monitors := &mockMonitorLister{ids: []int64{1}}

	job := NewPublishDailyTopicsJob(
		monitors,
		&mockDigestService{},
		&mockLLMClient{},
		&mockObsidianWriter{},
		exports,
		sched,
		PublishDailyTopicsConfig{VaultPath: "/tmp/vault", Target: "yesterday", TopN: 20},
	)

	err := job.Run(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exports.upsertCalls) != 0 {
		t.Fatalf("expected no upsert calls, got %d", len(exports.upsertCalls))
	}
}

func TestPublishDailyTopics_PublishesTopics(t *testing.T) {
	// 09:00 CST — after gate, first run today
	now := time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC)
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")

	topics := []TopicCandidate{
		{
			TopicID:        1,
			TopicKey:       "ai:监管",
			Title:          "AI 监管政策",
			HeatScore:      85.5,
			TrendDirection: "rising",
			PostCount:      12,
			Posts: []RepresentativePost{
				{AuthorName: "user1", Text: "post text", URL: "https://x.com/1"},
			},
		},
	}

	exports := &mockExportRepo{lastRunDate: ""}
	monitors := &mockMonitorLister{ids: []int64{1}}

	job := NewPublishDailyTopicsJob(
		monitors,
		&mockDigestService{topics: topics},
		&mockLLMClient{summary: "AI 监管摘要"},
		&mockObsidianWriter{path: "/tmp/vault/HotKey/topics/ai/2026-06-13-topic-1.md"},
		exports,
		sched,
		PublishDailyTopicsConfig{VaultPath: "/tmp/vault", Target: "yesterday", TopN: 20},
	)

	err := job.Run(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 upsert calls: pending + published
	if len(exports.upsertCalls) != 2 {
		t.Fatalf("expected 2 upsert calls, got %d", len(exports.upsertCalls))
	}
	if exports.upsertCalls[0].Status != "pending" {
		t.Fatalf("first upsert: expected status=pending, got %s", exports.upsertCalls[0].Status)
	}
	if exports.upsertCalls[1].Status != "published" {
		t.Fatalf("second upsert: expected status=published, got %s", exports.upsertCalls[1].Status)
	}
	if exports.upsertCalls[1].MarkdownPath == "" {
		t.Fatal("second upsert: expected non-empty markdown path")
	}
}

func TestPublishDailyTopics_LLMFailureDoesNotBlockOtherTopics(t *testing.T) {
	now := time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC)
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")

	topics := []TopicCandidate{
		{TopicID: 1, TopicKey: "a", Title: "Topic A", HeatScore: 90},
		{TopicID: 2, TopicKey: "b", Title: "Topic B", HeatScore: 80},
	}

	llmMock := &failingLLMClient{failOnTopic: 1}

	exports := &mockExportRepo{lastRunDate: ""}
	monitors := &mockMonitorLister{ids: []int64{1}}

	job := NewPublishDailyTopicsJob(
		monitors,
		&mockDigestService{topics: topics},
		llmMock,
		&mockObsidianWriter{path: "/tmp/vault/topic.md"},
		exports,
		sched,
		PublishDailyTopicsConfig{VaultPath: "/tmp/vault", Target: "yesterday", TopN: 20},
	)

	err := job.Run(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Topic 1: 1 upsert (failed)
	// Topic 2: 2 upserts (pending + published)
	totalUpserts := len(exports.upsertCalls)
	if totalUpserts != 3 {
		t.Fatalf("expected 3 upsert calls, got %d", totalUpserts)
	}

	// First call should be failed for topic 1
	if exports.upsertCalls[0].Status != "failed" {
		t.Fatalf("first upsert: expected status=failed, got %s", exports.upsertCalls[0].Status)
	}
	// Third call should be published for topic 2
	if exports.upsertCalls[2].Status != "published" {
		t.Fatalf("third upsert: expected status=published, got %s", exports.upsertCalls[2].Status)
	}
}

type failingLLMClient struct {
	failOnTopic int64
	callCount   int
}

func (m *failingLLMClient) SummarizeTopic(_ context.Context, in TopicSummaryInput) (string, error) {
	m.callCount++
	// Fail on first call by checking topic key
	if m.callCount == 1 {
		return "", errors.New("llm timeout")
	}
	return "summary", nil
}

func TestPublishDailyTopics_IdempotentPublish(t *testing.T) {
	// First run
	now := time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC)
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")

	topics := []TopicCandidate{
		{TopicID: 1, TopicKey: "a", Title: "Topic A", HeatScore: 90},
	}

	exports := &mockExportRepo{lastRunDate: ""}
	monitors := &mockMonitorLister{ids: []int64{1}}

	job := NewPublishDailyTopicsJob(
		monitors,
		&mockDigestService{topics: topics},
		&mockLLMClient{summary: "summary"},
		&mockObsidianWriter{path: "/tmp/vault/topic.md"},
		exports,
		sched,
		PublishDailyTopicsConfig{VaultPath: "/tmp/vault", Target: "yesterday", TopN: 20},
	)

	err := job.Run(context.Background(), now)
	if err != nil {
		t.Fatalf("first run: unexpected error: %v", err)
	}

	// Second run same day — should be skipped
	exports.lastRunDate = "2026-06-14"
	exports.upsertCalls = nil

	err = job.Run(context.Background(), now)
	if err != nil {
		t.Fatalf("second run: unexpected error: %v", err)
	}
	if len(exports.upsertCalls) != 0 {
		t.Fatalf("second run: expected 0 upsert calls, got %d", len(exports.upsertCalls))
	}
}

func TestPublishDailyTopics_GetLastRunDateError(t *testing.T) {
	now := time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC)
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")

	exports := &mockExportRepo{getErr: errors.New("db down")}
	monitors := &mockMonitorLister{ids: []int64{1}}

	job := NewPublishDailyTopicsJob(
		monitors,
		&mockDigestService{},
		&mockLLMClient{},
		&mockObsidianWriter{},
		exports,
		sched,
		PublishDailyTopicsConfig{VaultPath: "/tmp/vault"},
	)

	err := job.Run(context.Background(), now)
	if err == nil {
		t.Fatal("expected error from GetLastRunDate")
	}
}

func TestResolveExportDate_Yesterday(t *testing.T) {
	// 2026-06-14 12:00 CST = 2026-06-14 04:00 UTC
	now := time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)
	d := ResolveExportDate(now, "yesterday")
	want := time.Date(2026, 6, 13, 0, 0, 0, 0, d.Location())
	if d != want {
		t.Fatalf("got %v, want %v", d, want)
	}
}

func TestResolveExportDate_Today(t *testing.T) {
	now := time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)
	d := ResolveExportDate(now, "today")
	want := time.Date(2026, 6, 14, 0, 0, 0, 0, d.Location())
	if d != want {
		t.Fatalf("got %v, want %v", d, want)
	}
}
