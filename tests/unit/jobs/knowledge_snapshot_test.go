package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/jobs"
)

type mockDigestService struct {
	Result jobs.DigestResult
	Err    error
}

func (m *mockDigestService) BuildDigest(ctx context.Context, now time.Time) (jobs.DigestResult, error) {
	return m.Result, m.Err
}

type mockEventAssembler struct {
	Events []jobs.EventSnapshot
	Err    error
}

func (m *mockEventAssembler) BuildEvents(ctx context.Context, topics []jobs.TopicDigest) ([]jobs.EventSnapshot, error) {
	return m.Events, m.Err
}

type mockKnowledgeExporter struct {
	Result jobs.KnowledgeRunResult
	Err    error
}

func (m *mockKnowledgeExporter) Publish(ctx context.Context, digest jobs.DigestResult, events []jobs.EventSnapshot) (jobs.KnowledgeRunResult, error) {
	return m.Result, m.Err
}

func TestPublishKnowledgeSnapshotJob_Run(t *testing.T) {
	job := jobs.NewPublishKnowledgeSnapshotJob(
		&mockDigestService{
			Result: jobs.DigestResult{Topics: []jobs.TopicDigest{{ID: 1, Title: "AI 监管"}}},
		},
		&mockEventAssembler{
			Events: []jobs.EventSnapshot{{ID: 101, Title: "AI 监管新规发布"}},
		},
		&mockKnowledgeExporter{
			Result: jobs.KnowledgeRunResult{EventsPublished: 1},
		},
	)
	result, err := job.Run(context.Background(), time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("run job: %v", err)
	}
	if result.EventsPublished == 0 {
		t.Fatal("expected events to be published")
	}
}
