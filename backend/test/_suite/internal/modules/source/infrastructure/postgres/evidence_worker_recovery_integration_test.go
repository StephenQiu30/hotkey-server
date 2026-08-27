//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/jobs"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

const (
	observationCrashFlag        = "HOTKEY_TEST_OBSERVATION_WORKER_CRASH"
	observationCrashDSNFlag     = "HOTKEY_TEST_OBSERVATION_WORKER_DSN"
	observationCrashCommandFlag = "HOTKEY_TEST_OBSERVATION_WORKER_COMMAND"
	observationCrashExitCode    = 93
)

func TestEvidenceWorkerKillAfterObservationCommitRecoversOneDocumentJob(t *testing.T) {
	ctx := context.Background()
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := newRecoverableEvidenceRepository(t, runtime)
	fixture := newEvidenceRepositoryFixture(t, runtime.SQL, "worker-crash-after-observation")
	persisted, err := repository.Reserve(ctx, fixture.Reservation)
	if err != nil {
		t.Fatal(err)
	}
	command := sourceapplication.CommitEvidenceSnapshotCommand{
		SnapshotID: persisted.ID, StoreResult: storeResult(persisted),
		Observations: []sourceapplication.SourceObservationDTO{
			evidenceObservation(fixture, "worker-crash-observation", "Worker crash observation", digestValue("worker crash selected payload")),
		},
		DocumentGenerationScheduledAt: time.Now().UTC().Add(time.Hour),
		TraceID:                       "0123456789abcdef0123456789abcdef",
	}
	encoded, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	jobs := queue.NewStore(runtime)
	jobID, created, err := jobs.Enqueue(ctx, queue.Job{
		Kind: queue.KindRunRetention, UniqueKey: "worker-crash-after-observation-commit",
		Payload: queue.Payload{EntityID: persisted.ID, EntityVersion: 1}, ScheduledAt: time.Now().UTC().Add(-time.Minute),
		MaxAttempts: 3, Priority: 1,
	})
	if err != nil || !created {
		t.Fatalf("Enqueue() = %d/%t/%v", jobID, created, err)
	}

	process := exec.Command(os.Args[0], "-test.run=^TestEvidenceWorkerCrashAfterObservationCommitHelper$")
	process.Env = append(os.Environ(),
		observationCrashFlag+"=1",
		observationCrashDSNFlag+"="+runtime.Pool.Config().ConnString(),
		observationCrashCommandFlag+"="+string(encoded),
	)
	output, err := process.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != observationCrashExitCode {
		t.Fatalf("crash worker exit = %v, output=%s", err, output)
	}

	assertObservationCrashFacts(t, runtime, persisted.ID, fixture.SourceID, jobID, "running", 1)
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET attempted_at=now()-interval '2 minutes' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	recovered := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindRunRetention: func(ctx context.Context, _ queue.Job) error {
		_, err := repository.Commit(ctx, command)
		return err
	}})
	if reclaimed, err := recovered.ReclaimStale(ctx, time.Minute); err != nil || reclaimed != 1 {
		t.Fatalf("ReclaimStale() = %d/%v", reclaimed, err)
	}
	if worked, err := recovered.RunOnce(ctx); err != nil || !worked {
		t.Fatalf("RunOnce(recovered) = %t/%v", worked, err)
	}
	assertObservationCrashFacts(t, runtime, persisted.ID, fixture.SourceID, jobID, "completed", 2)
	var leaseFailures int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job_attempt WHERE job_id=$1 AND error='lease_expired'`, jobID).Scan(&leaseFailures); err != nil {
		t.Fatal(err)
	}
	if leaseFailures != 1 {
		t.Fatalf("lease expiry facts = %d, want 1", leaseFailures)
	}
}

func TestEvidenceWorkerCrashAfterObservationCommitHelper(t *testing.T) {
	if os.Getenv(observationCrashFlag) != "1" {
		return
	}
	var command sourceapplication.CommitEvidenceSnapshotCommand
	if err := json.Unmarshal([]byte(os.Getenv(observationCrashCommandFlag)), &command); err != nil {
		t.Fatal(err)
	}
	runtime, err := database.Open(context.Background(), os.Getenv(observationCrashDSNFlag))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	repository := newRecoverableEvidenceRepository(t, runtime)
	worker := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindRunRetention: func(ctx context.Context, _ queue.Job) error {
		if _, err := repository.Commit(ctx, command); err != nil {
			return err
		}
		os.Exit(observationCrashExitCode)
		return nil
	}})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(crash helper) = %t/%v", worked, err)
	}
	t.Fatal("crash helper returned without terminating")
}

func newRecoverableEvidenceRepository(t *testing.T, runtime *database.Runtime) *sourcepostgres.EvidenceSnapshotRepository {
	t.Helper()
	scheduler, err := sourcejobs.NewSourceDocumentGenerationScheduler(queue.NewStore(runtime))
	if err != nil {
		t.Fatal(err)
	}
	repository, err := sourcepostgres.NewEvidenceSnapshotRepository(runtime, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func assertObservationCrashFacts(t *testing.T, runtime *database.Runtime, snapshotID, sourceID, jobID int64, wantJobState string, wantAttempt int) {
	t.Helper()
	var lifecycle, jobState string
	var observations, references, documentJobs, attempt int
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state FROM evidence_snapshots WHERE id=$1`, snapshotID).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_observations WHERE source_connection_id=$1`, sourceID).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_observation_evidences WHERE evidence_snapshot_id=$1`, snapshotID).Scan(&references); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM river_job WHERE kind=$1`, queue.KindGenerateSourceDocument).Scan(&documentJobs); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT state,attempt FROM river_job WHERE id=$1`, jobID).Scan(&jobState, &attempt); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "raw_available" || observations != 1 || references != 1 || documentJobs != 1 || jobState != wantJobState || attempt != wantAttempt {
		t.Fatalf("observation recovery facts = lifecycle %q observations/references/jobs %d/%d/%d owner %q/%d",
			lifecycle, observations, references, documentJobs, jobState, attempt)
	}
}
