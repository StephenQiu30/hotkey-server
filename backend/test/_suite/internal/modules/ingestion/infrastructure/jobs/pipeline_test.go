package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func TestPipelineHandlersRejectInvalidEnvelopeBeforeDependencies(t *testing.T) {
	job := queue.Job{Kind: queue.KindEvaluateRelevance, UniqueKey: "x", ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1, Payload: queue.Payload{EntityID: 1, EntityVersion: 1}}
	if err := (&NormalizeHandler{}).Handle(context.Background(), job); !queue.IsPermanent(err) {
		t.Fatalf("normalize invalid kind = %v", err)
	}
	wrongKind := job
	wrongKind.Kind = queue.KindClusterContent
	if err := (&EvaluateHandler{}).Handle(context.Background(), wrongKind); !queue.IsPermanent(err) {
		t.Fatalf("evaluate invalid kind = %v", err)
	}
}

func TestNormalizeHandlerDrainsEveryCapturedItemPage(t *testing.T) {
	t.Parallel()

	service := &pagedNormalizeService{pages: []ingestionapplication.IngestRunResult{
		{Processed: 50, Bound: 50, NextCursor: "more"},
		{Processed: 50, Bound: 50},
	}}
	jobs := &recordingJobEnqueuer{}
	handler, err := newNormalizeHandler(service, jobs)
	if err != nil {
		t.Fatalf("newNormalizeHandler() error = %v", err)
	}
	job := queue.Job{
		Kind: queue.KindNormalizeContent, UniqueKey: "normalize", ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 2,
		Payload: queue.Payload{EntityID: 18, EntityVersion: 1, InputHash: strings.Repeat("a", 64)},
	}
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if service.calls != 2 || len(jobs.jobs) != 100 {
		t.Fatalf("drain calls/jobs = %d/%d, want 2/100", service.calls, len(jobs.jobs))
	}
}

func TestEvaluateHandlerSkipsClusterOnlyForStaleContent(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 7, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name        string
		publishedAt time.Time
		wantJobs    int
	}{
		{name: "inclusive boundary", publishedAt: now.Add(-ingestiondomain.NewEventFreshnessWindow), wantJobs: 1},
		{name: "stale background", publishedAt: now.Add(-ingestiondomain.NewEventFreshnessWindow - time.Nanosecond), wantJobs: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := ingestiondomain.Content{
				ID: 7, Version: 2, SourceConnectionID: 3, ContentType: "article", Title: "evidence",
				CanonicalURL: "https://example.test/evidence", Language: "en", PublishedAt: test.publishedAt,
				FetchedAt: now, ContentHash: strings.Repeat("a", 64), Status: ingestiondomain.ContentStatusActive,
			}
			candidates, err := ingestionapplication.NewCandidateRecallService(emptyRelevanceCandidateReader{}, nil)
			if err != nil {
				t.Fatalf("NewCandidateRecallService() error = %v", err)
			}
			jobs := &recordingJobEnqueuer{}
			handler, err := NewEvaluateHandlerWithClock(contentRepositoryFake{content: content}, candidates, relevanceRepositoryFake{}, jobs, func() time.Time { return now })
			if err != nil {
				t.Fatalf("NewEvaluateHandlerWithClock() error = %v", err)
			}
			job := queue.Job{
				Kind: queue.KindEvaluateRelevance, UniqueKey: "evaluate", ScheduledAt: now, MaxAttempts: 3, Priority: 3,
				Payload: queue.Payload{EntityID: content.ID, EntityVersion: content.Version, InputHash: strings.Repeat("b", 64)},
			}
			if err := handler.Handle(context.Background(), job); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if len(jobs.jobs) != test.wantJobs {
				t.Fatalf("cluster jobs = %d, want %d", len(jobs.jobs), test.wantJobs)
			}
			if len(jobs.jobs) == 1 && jobs.jobs[0].Kind != queue.KindClusterContent {
				t.Fatalf("job kind = %q, want %q", jobs.jobs[0].Kind, queue.KindClusterContent)
			}
		})
	}
}

type contentRepositoryFake struct{ content ingestiondomain.Content }

func (repository contentRepositoryFake) GetActive(context.Context, int64) (ingestiondomain.Content, error) {
	return repository.content, nil
}

type emptyRelevanceCandidateReader struct{}

func (emptyRelevanceCandidateReader) SourceCandidates(context.Context, int64, int) ([]ingestiondomain.RelevanceCandidateHit, error) {
	return []ingestiondomain.RelevanceCandidateHit{}, nil
}
func (emptyRelevanceCandidateReader) LexicalCandidates(context.Context, []string, int) ([]ingestiondomain.RelevanceCandidateHit, error) {
	return []ingestiondomain.RelevanceCandidateHit{}, nil
}
func (emptyRelevanceCandidateReader) LoadRelevanceCandidates(context.Context, []int64) ([]ingestiondomain.RelevanceCandidate, error) {
	return []ingestiondomain.RelevanceCandidate{}, nil
}

type relevanceRepositoryFake struct{}

func (relevanceRepositoryFake) UpsertSnapshot(context.Context, ingestiondomain.RelevanceSnapshotInput) (ingestiondomain.RelevanceSnapshot, bool, error) {
	return ingestiondomain.RelevanceSnapshot{}, false, errors.New("unexpected relevance snapshot")
}

type recordingJobEnqueuer struct{ jobs []queue.Job }

func (enqueuer *recordingJobEnqueuer) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	enqueuer.jobs = append(enqueuer.jobs, job)
	return int64(len(enqueuer.jobs)), true, nil
}

type pagedNormalizeService struct {
	pages []ingestionapplication.IngestRunResult
	calls int
}

func (service *pagedNormalizeService) IngestRunWithHook(_ context.Context, _ ingestionapplication.IngestRunInput, hook func(context.Context, int64) error) (ingestionapplication.IngestRunResult, error) {
	result := service.pages[service.calls]
	service.calls++
	for offset := 0; offset < result.Bound; offset++ {
		if err := hook(context.Background(), int64((service.calls-1)*50+offset+1)); err != nil {
			return ingestionapplication.IngestRunResult{}, err
		}
	}
	return result, nil
}
