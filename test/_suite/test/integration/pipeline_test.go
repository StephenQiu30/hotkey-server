//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

type pipelineFixture struct {
	Version string `json:"version"`
	Sources []struct {
		ID             int64  `json:"id"`
		Kind           string `json:"kind"`
		QuerySignature string `json:"query_signature"`
		ContentID      int64  `json:"content_id"`
		EventID        int64  `json:"event_id"`
		Failure        string `json:"failure"`
	} `json:"sources"`
	Summary struct {
		Status     string `json:"status"`
		ReasonCode string `json:"reason_code"`
	} `json:"summary"`
}

type pipelineAcceptance struct {
	runtime *database.Runtime
	store   *queue.Store
	fixture pipelineFixture
	sources map[int64]struct {
		contentID int64
		eventID   int64
		failure   string
	}
	contentEvents map[int64]int64
}

func TestRSSHNPipelineRecovery(t *testing.T) {
	ctx := context.Background()
	fixture := readPipelineFixture(t)
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
CREATE TABLE pipeline_fixture_facts (
	stage text NOT NULL,
	entity_id bigint NOT NULL,
	detail text NOT NULL,
	PRIMARY KEY (stage, entity_id)
)`); err != nil {
		t.Fatal(err)
	}

	pipeline := newPipelineAcceptance(runtime, fixture)
	worker := queue.NewWorker(runtime, pipeline.handlers())
	now := time.Now().UTC().Truncate(time.Microsecond)
	var recoveredJobID int64
	for _, source := range fixture.Sources {
		job := pipeline.collectJob(source.ID, source.QuerySignature, now)
		firstID, created, err := pipeline.store.Enqueue(ctx, job)
		if err != nil || !created || firstID == 0 {
			t.Fatalf("enqueue %s source %d = %d/%t/%v", source.Kind, source.ID, firstID, created, err)
		}
		if source.ID == fixture.Sources[0].ID {
			recoveredJobID = firstID
			duplicateID, duplicateCreated, err := pipeline.store.Enqueue(ctx, job)
			if err != nil || duplicateID != firstID || duplicateCreated {
				t.Fatalf("duplicate time slice = %d/%t/%v, want %d/false/nil", duplicateID, duplicateCreated, err, firstID)
			}
		}
	}

	// Simulate a process exit after claim. Recovery must make the stale lease
	// available before the next worker continues the same pipeline.
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET state = 'running', attempt = 1, attempted_at = now() - interval '2 minutes' WHERE id = $1`, recoveredJobID); err != nil {
		t.Fatal(err)
	}
	if reclaimed, err := worker.ReclaimStale(ctx, time.Minute); err != nil || reclaimed != 1 {
		t.Fatalf("reclaim stale pipeline job = %d/%v, want 1/nil", reclaimed, err)
	}
	drainPipeline(t, ctx, worker)

	for _, stage := range []string{
		queue.KindCollectSource,
		queue.KindNormalizeContent,
		queue.KindEvaluateRelevance,
		queue.KindClusterContent,
		queue.KindRecomputeEventHeat,
		queue.KindGenerateEventSummary,
	} {
		if got := pipeline.countFacts(ctx, stage); got != 2 {
			t.Fatalf("stage %q facts = %d, want two successful RSS/HN facts", stage, got)
		}
	}
	var summaryDetail string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT detail FROM pipeline_fixture_facts WHERE stage = $1 ORDER BY entity_id LIMIT 1`, queue.KindGenerateEventSummary).Scan(&summaryDetail); err != nil {
		t.Fatal(err)
	}
	wantDetail := fixture.Summary.Status + ":" + fixture.Summary.ReasonCode
	if summaryDetail != wantDetail {
		t.Fatalf("summary detail = %q, want degraded fixture %q", summaryDetail, wantDetail)
	}

	var failedState string
	failedKey := pipeline.collectJob(fixture.Sources[2].ID, fixture.Sources[2].QuerySignature, now).UniqueKey
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE unique_key = $1`, []byte(failedKey)).Scan(&failedState); err != nil {
		t.Fatal(err)
	}
	if failedState != "available" {
		t.Fatalf("failed source state = %q, want available for retry", failedState)
	}
	if got := pipeline.countFactsForEntity(ctx, queue.KindCollectSource, fixture.Sources[2].ContentID); got != 0 {
		t.Fatalf("failed source produced %d capture facts", got)
	}
}

func (pipeline *pipelineAcceptance) handlers() map[string]queue.Handler {
	return map[string]queue.Handler{
		queue.KindCollectSource:        pipeline.collect,
		queue.KindNormalizeContent:     pipeline.normalize,
		queue.KindEvaluateRelevance:    pipeline.evaluate,
		queue.KindClusterContent:       pipeline.cluster,
		queue.KindRecomputeEventHeat:   pipeline.heat,
		queue.KindGenerateEventSummary: pipeline.summary,
	}
}

func (pipeline *pipelineAcceptance) collect(ctx context.Context, job queue.Job) error {
	source, ok := pipeline.sources[job.Payload.EntityID]
	if !ok {
		return queue.NewPermanentError(fmt.Errorf("unknown fixture source %d", job.Payload.EntityID))
	}
	if source.failure != "" {
		return queue.NewRetryableError(errors.New(source.failure))
	}
	if err := pipeline.record(ctx, queue.KindCollectSource, source.contentID, "captured"); err != nil {
		return err
	}
	return pipeline.enqueue(ctx, queue.KindNormalizeContent, source.contentID, job)
}

func (pipeline *pipelineAcceptance) normalize(ctx context.Context, job queue.Job) error {
	if err := pipeline.record(ctx, queue.KindNormalizeContent, job.Payload.EntityID, "normalized"); err != nil {
		return err
	}
	return pipeline.enqueue(ctx, queue.KindEvaluateRelevance, job.Payload.EntityID, job)
}

func (pipeline *pipelineAcceptance) evaluate(ctx context.Context, job queue.Job) error {
	if err := pipeline.record(ctx, queue.KindEvaluateRelevance, job.Payload.EntityID, "matched"); err != nil {
		return err
	}
	return pipeline.enqueue(ctx, queue.KindClusterContent, job.Payload.EntityID, job)
}

func (pipeline *pipelineAcceptance) cluster(ctx context.Context, job queue.Job) error {
	if err := pipeline.record(ctx, queue.KindClusterContent, job.Payload.EntityID, "clustered"); err != nil {
		return err
	}
	eventID, ok := pipeline.contentEvents[job.Payload.EntityID]
	if !ok {
		return queue.NewPermanentError(fmt.Errorf("unknown fixture event for content %d", job.Payload.EntityID))
	}
	return pipeline.enqueue(ctx, queue.KindRecomputeEventHeat, eventID, job)
}

func (pipeline *pipelineAcceptance) heat(ctx context.Context, job queue.Job) error {
	if err := pipeline.record(ctx, queue.KindRecomputeEventHeat, job.Payload.EntityID, "heat_recomputed"); err != nil {
		return err
	}
	return pipeline.enqueue(ctx, queue.KindGenerateEventSummary, job.Payload.EntityID, job)
}

func (pipeline *pipelineAcceptance) summary(ctx context.Context, job queue.Job) error {
	detail := pipeline.fixture.Summary.Status + ":" + pipeline.fixture.Summary.ReasonCode
	return pipeline.record(ctx, queue.KindGenerateEventSummary, job.Payload.EntityID, detail)
}

func (pipeline *pipelineAcceptance) enqueue(ctx context.Context, kind string, entityID int64, previous queue.Job) error {
	hash := queue.StableJobHash(kind, fmt.Sprint(entityID), pipeline.fixture.Version)
	_, _, err := pipeline.store.Enqueue(ctx, queue.Job{
		Kind:      kind,
		UniqueKey: queue.StableJobKey(kind, entityID, 1, hash),
		Payload: queue.Payload{
			EntityID: entityID, EntityVersion: 1,
			WindowStart: previous.Payload.WindowStart, WindowEnd: previous.Payload.WindowEnd,
			InputHash: hash,
		},
		ScheduledAt: previous.ScheduledAt, MaxAttempts: 3, Priority: previous.Priority + 1,
	})
	return err
}

func (pipeline *pipelineAcceptance) collectJob(sourceID int64, signature string, now time.Time) queue.Job {
	return queue.Job{
		Kind:      queue.KindCollectSource,
		UniqueKey: fmt.Sprintf("fixture:collect:%d:%s", sourceID, signature),
		Payload: queue.Payload{
			EntityID: sourceID, EntityVersion: 1,
			WindowStart: now.Add(-5 * time.Minute), WindowEnd: now, InputHash: signature,
		},
		ScheduledAt: now.Add(-time.Second), MaxAttempts: 3, Priority: 1,
	}
}

func (pipeline *pipelineAcceptance) record(ctx context.Context, stage string, entityID int64, detail string) error {
	_, err := pipeline.runtime.SQL.ExecContext(ctx, `
INSERT INTO pipeline_fixture_facts (stage, entity_id, detail)
VALUES ($1, $2, $3)
ON CONFLICT (stage, entity_id) DO NOTHING`, stage, entityID, detail)
	return err
}

func (pipeline *pipelineAcceptance) countFacts(ctx context.Context, stage string) int {
	var count int
	if err := pipeline.runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_fixture_facts WHERE stage = $1`, stage).Scan(&count); err != nil {
		panic(err)
	}
	return count
}

func (pipeline *pipelineAcceptance) countFactsForEntity(ctx context.Context, stage string, entityID int64) int {
	var count int
	if err := pipeline.runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM pipeline_fixture_facts WHERE stage = $1 AND entity_id = $2`, stage, entityID).Scan(&count); err != nil {
		panic(err)
	}
	return count
}

func newPipelineAcceptance(runtime *database.Runtime, fixture pipelineFixture) *pipelineAcceptance {
	pipeline := &pipelineAcceptance{
		runtime: runtime,
		store:   queue.NewStore(runtime),
		fixture: fixture,
		sources: make(map[int64]struct {
			contentID int64
			eventID   int64
			failure   string
		}),
		contentEvents: make(map[int64]int64),
	}
	for _, source := range fixture.Sources {
		pipeline.sources[source.ID] = struct {
			contentID int64
			eventID   int64
			failure   string
		}{contentID: source.ContentID, eventID: source.EventID, failure: source.Failure}
		pipeline.contentEvents[source.ContentID] = source.EventID
	}
	return pipeline
}

func drainPipeline(t *testing.T, ctx context.Context, worker *queue.Worker) {
	t.Helper()
	for attempt := 0; attempt < 32; attempt++ {
		claimed, err := worker.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !claimed {
			return
		}
	}
	t.Fatal("pipeline did not drain within 32 jobs")
}

func readPipelineFixture(t *testing.T) pipelineFixture {
	t.Helper()
	root := findRepositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "test", "fixtures", "pipeline", "v1", "pipeline.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture pipelineFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version == "" || len(fixture.Sources) != 3 || fixture.Summary.Status != "degraded" {
		t.Fatalf("invalid pipeline fixture: %#v", fixture)
	}
	return fixture
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(directory, "test", "fixtures", "pipeline", "v1", "pipeline.json")
		if _, err := os.Stat(candidate); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	t.Fatal("repository root not found")
	return ""
}
