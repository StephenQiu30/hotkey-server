package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/jobs"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func TestEvidenceSnapshotCommitSchedulesEveryExactReferenceIdempotently(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	scheduler, err := sourcejobs.NewSourceDocumentGenerationScheduler(queue.NewStore(runtime))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourcepostgres.NewEvidenceSnapshotRepository(runtime, nil); err == nil {
		t.Fatal("NewEvidenceSnapshotRepository() accepted a missing production scheduler")
	}
	repository, err := sourcepostgres.NewEvidenceSnapshotRepository(runtime, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "document-scheduling")
	persisted, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	first := evidenceObservation(fixture, "scheduled-entry-one", "First", digestValue("selected one"))
	second := evidenceObservation(fixture, "scheduled-entry-two", "Second", digestValue("selected two"))
	scheduledAt := time.Date(2026, time.August, 9, 14, 0, 0, 0, time.UTC)
	traceID := "0123456789abcdef0123456789abcdef"
	commit := sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: persisted.ID, StoreResult: storeResult(persisted),
		Observations:                  []sourceapplication.SourceObservationDTO{first, second},
		DocumentGenerationScheduledAt: scheduledAt,
		TraceID:                       traceID,
	}
	committed, err := repository.Commit(ctx, commit)
	if err != nil {
		t.Fatalf("Commit(): %v", err)
	}
	if committed.Snapshot.ID != persisted.ID || len(committed.EvidenceReferences) != 2 {
		t.Fatalf("Commit() = %#v", committed)
	}
	for _, receipt := range committed.EvidenceReferences {
		if receipt.EvidenceReferenceID <= 0 || receipt.SourceObservationID <= 0 || receipt.EvidenceSnapshotID != persisted.ID {
			t.Fatalf("exact reference receipt = %#v", receipt)
		}
	}
	jobs := readSourceDocumentJobs(t, runtime.SQL)
	if len(jobs) != 2 {
		t.Fatalf("source document jobs = %d, want 2", len(jobs))
	}
	for index, job := range jobs {
		referenceID := committed.EvidenceReferences[index].EvidenceReferenceID
		if job.EvidenceReferenceID != referenceID || job.TraceID != traceID || job.ScheduledAt != scheduledAt ||
			job.UniqueKey != sourcejobs.SourceDocumentGenerationUniqueKey(referenceID) {
			t.Fatalf("job %d = %#v, reference=%#v", index, job, committed.EvidenceReferences[index])
		}
	}

	retry := commit
	retry.TraceID = "fedcba9876543210fedcba9876543210"
	retry.DocumentGenerationScheduledAt = scheduledAt.Add(time.Hour)
	replayed, err := repository.Commit(ctx, retry)
	if err != nil {
		t.Fatalf("available Commit() retry: %v", err)
	}
	if !reflect.DeepEqual(replayed.EvidenceReferences, committed.EvidenceReferences) {
		t.Fatalf("available retry receipts changed: before=%#v after=%#v", committed.EvidenceReferences, replayed.EvidenceReferences)
	}
	replayedJobs := readSourceDocumentJobs(t, runtime.SQL)
	if !reflect.DeepEqual(replayedJobs, jobs) {
		t.Fatalf("available retry changed durable jobs: before=%#v after=%#v", jobs, replayedJobs)
	}
	missingReferenceID := committed.EvidenceReferences[1].EvidenceReferenceID
	if _, err := runtime.SQL.ExecContext(ctx, `DELETE FROM river_job WHERE kind=$1 AND unique_key=$2`,
		queue.KindGenerateSourceDocument, []byte(sourcejobs.SourceDocumentGenerationUniqueKey(missingReferenceID))); err != nil {
		t.Fatal(err)
	}
	recoverMissingJob := commit
	recoverMissingJob.TraceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	recoverMissingJob.DocumentGenerationScheduledAt = scheduledAt.Add(90 * time.Minute)
	recovered, err := repository.Commit(ctx, recoverMissingJob)
	if err != nil {
		t.Fatalf("available Commit() missing-job recovery: %v", err)
	}
	if !reflect.DeepEqual(recovered.EvidenceReferences, committed.EvidenceReferences) {
		t.Fatalf("missing-job recovery receipts changed: before=%#v after=%#v", committed.EvidenceReferences, recovered.EvidenceReferences)
	}
	recoveredJobs := readSourceDocumentJobs(t, runtime.SQL)
	if len(recoveredJobs) != 2 || recoveredJobs[0] != jobs[0] || recoveredJobs[1].EvidenceReferenceID != missingReferenceID ||
		recoveredJobs[1].TraceID != recoverMissingJob.TraceID || recoveredJobs[1].ScheduledAt != recoverMissingJob.DocumentGenerationScheduledAt {
		t.Fatalf("available retry did not recreate only the missing exact job: before=%#v after=%#v", jobs, recoveredJobs)
	}

	secondFixture := addEvidenceIdentity(t, runtime.SQL, fixture, "document-scheduling-second-snapshot")
	secondPersisted, err := repository.Reserve(ctx, secondFixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	linkedAgain := first
	linkedAgain.Evidence.EvidenceKey = secondPersisted.EvidenceKey
	linkedAgain.Evidence.Usage = "context"
	linkedAgain.Evidence.SelectedPayloadSHA256 = digestValue("same observation selected from second snapshot")
	secondCommit, err := repository.Commit(ctx, sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: secondPersisted.ID, StoreResult: storeResult(secondPersisted),
		Observations:                  []sourceapplication.SourceObservationDTO{linkedAgain},
		DocumentGenerationScheduledAt: scheduledAt.Add(2 * time.Hour), TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("second snapshot Commit(): %v", err)
	}
	if len(secondCommit.EvidenceReferences) != 1 || secondCommit.EvidenceReferences[0].EvidenceSnapshotID != secondPersisted.ID ||
		secondCommit.EvidenceReferences[0].SourceObservationID != committed.EvidenceReferences[0].SourceObservationID ||
		secondCommit.EvidenceReferences[0].EvidenceReferenceID == committed.EvidenceReferences[0].EvidenceReferenceID ||
		secondCommit.EvidenceReferences[0].Usage != "context" {
		t.Fatalf("M:N exact receipt = first:%#v second:%#v", committed.EvidenceReferences[0], secondCommit.EvidenceReferences)
	}
	if jobs := readSourceDocumentJobs(t, runtime.SQL); len(jobs) != 2 {
		t.Fatalf("context evidence scheduled a source document job: jobs=%#v", jobs)
	}
	var persistedUsage string
	if err := runtime.SQL.QueryRowContext(ctx, `
SELECT usage FROM source_observation_evidences WHERE id=$1`,
		secondCommit.EvidenceReferences[0].EvidenceReferenceID,
	).Scan(&persistedUsage); err != nil {
		t.Fatal(err)
	}
	if persistedUsage != "context" {
		t.Fatalf("persisted context evidence usage = %q", persistedUsage)
	}
}

func TestEvidenceSnapshotCommitRollsBackFirstRealJobWhenLaterEnqueueFails(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	if _, err := runtime.SQL.ExecContext(ctx, `
CREATE OR REPLACE FUNCTION reject_second_source_document_job() RETURNS trigger AS $$
BEGIN
  IF NEW.kind = 'generate_source_document'
     AND EXISTS (SELECT 1 FROM river_job WHERE kind = 'generate_source_document') THEN
    RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'reject second source document job';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER reject_second_source_document_job
BEFORE INSERT ON river_job
FOR EACH ROW EXECUTE FUNCTION reject_second_source_document_job();`); err != nil {
		t.Fatalf("install queue failure trigger: %v", err)
	}
	scheduler, err := sourcejobs.NewSourceDocumentGenerationScheduler(queue.NewStore(runtime))
	if err != nil {
		t.Fatal(err)
	}
	repository, err := sourcepostgres.NewEvidenceSnapshotRepository(runtime, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "document-scheduling-rollback")
	persisted, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	commit := sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: persisted.ID, StoreResult: storeResult(persisted),
		Observations: []sourceapplication.SourceObservationDTO{
			evidenceObservation(fixture, "rollback-entry-one", "First", digestValue("rollback selected one")),
			evidenceObservation(fixture, "rollback-entry-two", "Second", digestValue("rollback selected two")),
		},
		DocumentGenerationScheduledAt: time.Now().UTC(),
	}
	if _, err := repository.Commit(ctx, commit); err == nil {
		t.Fatal("Commit() unexpectedly succeeded when second enqueue failed")
	}
	var lifecycle string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT lifecycle_state FROM evidence_snapshots WHERE id=$1`, persisted.ID).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "raw_pending" {
		t.Fatalf("rolled back lifecycle = %q, want raw_pending", lifecycle)
	}
	assertEvidenceFactCounts(t, runtime.SQL, fixture.SourceID, 0, 0)
	if jobs := readSourceDocumentJobs(t, runtime.SQL); len(jobs) != 0 {
		t.Fatalf("first queue insert escaped rollback: %#v", jobs)
	}

	if _, err := runtime.SQL.ExecContext(ctx, `DROP TRIGGER reject_second_source_document_job ON river_job`); err != nil {
		t.Fatal(err)
	}
	committed, err := repository.Commit(ctx, commit)
	if err != nil {
		t.Fatalf("Commit() after queue recovery: %v", err)
	}
	if len(committed.EvidenceReferences) != 2 || len(readSourceDocumentJobs(t, runtime.SQL)) != 2 {
		t.Fatalf("recovered Commit() = %#v", committed)
	}
}

type persistedSourceDocumentJob struct {
	ID                  int64
	EvidenceReferenceID int64
	TraceID             string
	UniqueKey           string
	ScheduledAt         time.Time
}

func readSourceDocumentJobs(t *testing.T, database interface {
	Query(string, ...any) (*sql.Rows, error)
}) []persistedSourceDocumentJob {
	t.Helper()
	rows, err := database.Query(`
SELECT id,args::text,convert_from(unique_key,'UTF8'),scheduled_at
FROM river_job WHERE kind=$1 ORDER BY id`, queue.KindGenerateSourceDocument)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	jobs := make([]persistedSourceDocumentJob, 0)
	for rows.Next() {
		var job persistedSourceDocumentJob
		var encoded string
		if err := rows.Scan(&job.ID, &encoded, &job.UniqueKey, &job.ScheduledAt); err != nil {
			t.Fatal(err)
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(encoded), &args); err != nil {
			t.Fatal(err)
		}
		if len(args) != 2 {
			t.Fatalf("durable args keys = %#v", args)
		}
		job.EvidenceReferenceID = int64(args["evidence_reference_id"].(float64))
		job.TraceID, _ = args["trace_id"].(string)
		for _, forbidden := range []string{"body", "raw", "payload", "object_key"} {
			if strings.Contains(encoded, forbidden) {
				t.Fatalf("durable args leaked %q: %s", forbidden, encoded)
			}
		}
		job.ScheduledAt = job.ScheduledAt.UTC()
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return jobs
}
