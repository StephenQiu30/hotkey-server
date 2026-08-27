//go:build integration

package application_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	ingestionminio "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/minio"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	miniosdk "github.com/minio/minio-go/v7"
)

const (
	minioCrashFlag     = "HOTKEY_TEST_MINIO_WORKER_CRASH"
	minioCrashDSNFlag  = "HOTKEY_TEST_MINIO_WORKER_DSN"
	minioCrashRunFlag  = "HOTKEY_TEST_MINIO_WORKER_RUN_ID"
	minioCrashExitCode = 92
)

func TestIngestWorkerKillAfterMinIOWriteRecoversWithoutDuplicateSideEffects(t *testing.T) {
	ctx := context.Background()
	runtime := openIngestionRuntime(t)
	defer func() { _ = runtime.Close() }()
	body := fmt.Sprintf("worker crash evidence %d", time.Now().UnixNano())
	runID, sourceID := seedCapturedRun(t, runtime, []sourcedomain.CapturedItem{
		capturedItem("worker-crash-minio", "article", "Worker crash evidence", body),
	})
	store, client, cfg := integrationEvidenceStore(t)
	cleanupEvidencePrefix(t, store, sourceID)
	t.Cleanup(func() { cleanupEvidencePrefix(t, store, sourceID) })

	jobs := queue.NewStore(runtime)
	jobID, created, err := jobs.Enqueue(ctx, queue.Job{
		Kind: queue.KindRunRetention, UniqueKey: "worker-crash-after-minio-write",
		Payload: queue.Payload{EntityID: runID, EntityVersion: 1}, ScheduledAt: time.Now().UTC().Add(-time.Minute),
		MaxAttempts: 3, Priority: 1,
	})
	if err != nil || !created {
		t.Fatalf("Enqueue() = %d/%t/%v", jobID, created, err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestIngestWorkerCrashAfterMinIOWriteHelper$")
	command.Env = append(os.Environ(),
		minioCrashFlag+"=1",
		minioCrashDSNFlag+"="+runtime.Pool.Config().ConnString(),
		minioCrashRunFlag+"="+strconv.FormatInt(runID, 10),
	)
	output, err := command.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != minioCrashExitCode {
		t.Fatalf("crash worker exit = %v, output=%s", err, output)
	}

	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	objectKey := ingestionminio.EvidenceObjectKey(sourceID, digest)
	if _, err := client.StatObject(ctx, cfg.Bucket, objectKey, miniosdk.StatObjectOptions{}); err != nil {
		t.Fatalf("Head object written before Worker death: %v", err)
	}
	var state string
	var contents, assets int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE id=$1`, jobID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM contents),(SELECT count(*) FROM content_assets)`).Scan(&contents, &assets); err != nil {
		t.Fatal(err)
	}
	if state != "running" || contents != 0 || assets != 0 {
		t.Fatalf("post-crash facts = job %q contents %d assets %d, want running/0/0", state, contents, assets)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET attempted_at=now()-interval '2 minutes' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}

	service := newWorkerRecoveryIngestionService(t, runtime, store)
	recovered := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindRunRetention: func(ctx context.Context, job queue.Job) error {
		result, err := service.IngestRun(ctx, ingestionapplication.IngestRunInput{RunID: job.Payload.EntityID, Limit: 1})
		if err != nil {
			return err
		}
		if result.Bound != 1 || result.Failed != 0 {
			return fmt.Errorf("unexpected recovered ingestion result: %#v", result)
		}
		return nil
	}})
	if reclaimed, err := recovered.ReclaimStale(ctx, time.Minute); err != nil || reclaimed != 1 {
		t.Fatalf("ReclaimStale() = %d/%v", reclaimed, err)
	}
	if worked, err := recovered.RunOnce(ctx); err != nil || !worked {
		t.Fatalf("RunOnce(recovered) = %t/%v", worked, err)
	}

	var bindings, objects, leaseFailures int
	var ingestionStatus string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT state FROM river_job WHERE id=$1`, jobID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM contents),(SELECT count(*) FROM content_assets),(SELECT count(*) FROM collection_run_items WHERE run_id=$1)`, runID).Scan(&contents, &assets, &bindings); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT ingestion_status FROM collection_run_items WHERE run_id=$1`, runID).Scan(&ingestionStatus); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM river_job_attempt WHERE job_id=$1 AND error='lease_expired'`, jobID).Scan(&leaseFailures); err != nil {
		t.Fatal(err)
	}
	receipts, err := store.ListPrefix(ctx, fmt.Sprintf("evidence/v1/%d/", sourceID))
	if err != nil {
		t.Fatal(err)
	}
	objects = len(receipts)
	if state != "completed" || contents != 1 || assets != 1 || bindings != 1 || objects != 1 || ingestionStatus != "succeeded" || leaseFailures != 1 {
		t.Fatalf("recovered facts = job %q contents/assets/bindings/objects %d/%d/%d/%d status %q lease %d",
			state, contents, assets, bindings, objects, ingestionStatus, leaseFailures)
	}
}

func TestIngestWorkerCrashAfterMinIOWriteHelper(t *testing.T) {
	if os.Getenv(minioCrashFlag) != "1" {
		return
	}
	runID, err := strconv.ParseInt(os.Getenv(minioCrashRunFlag), 10, 64)
	if err != nil || runID <= 0 {
		t.Fatalf("invalid crash run id: %q", os.Getenv(minioCrashRunFlag))
	}
	runtime, err := database.Open(context.Background(), os.Getenv(minioCrashDSNFlag))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	store, _, _ := integrationEvidenceStore(t)
	service := newWorkerRecoveryIngestionService(t, runtime, killAfterPutEvidenceStore{EvidenceStore: store})
	worker := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindRunRetention: func(ctx context.Context, queueJob queue.Job) error {
		_, err := service.IngestRun(ctx, ingestionapplication.IngestRunInput{RunID: runID, Limit: 1})
		return err
	}})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(crash helper) = %t/%v", worked, err)
	}
	t.Fatal("crash helper returned without terminating")
}

type killAfterPutEvidenceStore struct {
	ingestiondomain.EvidenceStore
}

func (store killAfterPutEvidenceStore) PutText(ctx context.Context, object ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	receipt, err := store.EvidenceStore.PutText(ctx, object)
	if err != nil {
		return ingestiondomain.EvidenceReceipt{}, err
	}
	os.Exit(minioCrashExitCode)
	return receipt, nil
}

func newWorkerRecoveryIngestionService(t *testing.T, runtime *database.Runtime, evidence ingestiondomain.EvidenceStore) *ingestionapplication.Service {
	t.Helper()
	service, err := ingestionapplication.NewService(ingestionapplication.Dependencies{
		Runtime: runtime, Captures: newCapturedItemReader(t, runtime), Contents: ingestionpostgres.NewContentRepository(runtime),
		Evidence: evidence, Markdown: passthroughMarkdownProjector{},
	})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	return service
}
