//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

const (
	projectionCrashFlag        = "HOTKEY_TEST_PROJECTION_WORKER_CRASH"
	projectionCrashDSNFlag     = "HOTKEY_TEST_PROJECTION_WORKER_DSN"
	projectionCrashCommandFlag = "HOTKEY_TEST_PROJECTION_WORKER_COMMAND"
	projectionCrashVaultFlag   = "HOTKEY_TEST_PROJECTION_WORKER_VAULT"
	projectionCrashExitCode    = 94
)

func TestDocumentProjectionWorkerKillAfterVaultWriteRecoversOneArtifact(t *testing.T) {
	ctx := context.Background()
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "worker-crash-projection", 84)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	vaultRoot := t.TempDir()
	profile := strings.Repeat("9", 64)
	content := []byte("# Archived\n\nWorker crash projection.\n")
	command := derivedArtifactProjectCommand(fixture, profile, content, storeDecisionID, retainDecisionID, nil)
	encoded, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	jobs := queue.NewStore(runtime)
	jobID, created, err := jobs.Enqueue(ctx, queue.Job{
		Kind: queue.KindRunRetention, UniqueKey: "worker-crash-after-vault-write",
		Payload: queue.Payload{EntityID: fixture.persisted.DocumentVersion.ID, EntityVersion: 1}, ScheduledAt: time.Now().UTC().Add(-time.Minute),
		MaxAttempts: 3, Priority: 1,
	})
	if err != nil || !created {
		t.Fatalf("Enqueue() = %d/%t/%v", jobID, created, err)
	}

	process := exec.Command(os.Args[0], "-test.run=^TestDocumentProjectionWorkerCrashAfterVaultWriteHelper$")
	process.Env = append(os.Environ(),
		projectionCrashFlag+"=1",
		projectionCrashDSNFlag+"="+runtime.Pool.Config().ConnString(),
		projectionCrashCommandFlag+"="+string(encoded),
		projectionCrashVaultFlag+"="+vaultRoot,
	)
	output, err := process.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != projectionCrashExitCode {
		t.Fatalf("crash worker exit = %v, output=%s", err, output)
	}

	relativePath := derivedArtifactFixturePath(fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, profile)
	assertProjectionFile(t, vaultRoot, relativePath, content)
	assertProjectionCrashFacts(t, runtime, fixture.persisted.DocumentVersion.ID, jobID, "derive_pending", false, "running", 1)
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job SET attempted_at=now()-interval '2 minutes' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}

	saga := newDerivedArtifactSaga(t, runtime, newKnowledgeProjectionPublisher(t, vaultRoot), fixture.documentVersions)
	recovered := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindRunRetention: func(ctx context.Context, _ queue.Job) error {
		_, err := saga.Project(ctx, command)
		return err
	}})
	if reclaimed, err := recovered.ReclaimStale(ctx, time.Minute); err != nil || reclaimed != 1 {
		t.Fatalf("ReclaimStale() = %d/%v", reclaimed, err)
	}
	if worked, err := recovered.RunOnce(ctx); err != nil || !worked {
		t.Fatalf("RunOnce(recovered) = %t/%v", worked, err)
	}
	assertProjectionFile(t, vaultRoot, relativePath, content)
	assertProjectionCrashFacts(t, runtime, fixture.persisted.DocumentVersion.ID, jobID, "derived_available", true, "completed", 2)
	var documentState string
	var artifactCount, leaseFailures int
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state FROM document_versions WHERE id=$1`, fixture.persisted.DocumentVersion.ID).Scan(&documentState); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM derived_artifacts WHERE document_version_id=$1`, fixture.persisted.DocumentVersion.ID).Scan(&artifactCount); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM river_job_attempt WHERE job_id=$1 AND error='lease_expired'`, jobID).Scan(&leaseFailures); err != nil {
		t.Fatal(err)
	}
	if documentState != "derived_available" || artifactCount != 1 || leaseFailures != 1 {
		t.Fatalf("recovered projection facts = document %q artifacts %d lease %d", documentState, artifactCount, leaseFailures)
	}
}

func TestDocumentProjectionWorkerCrashAfterVaultWriteHelper(t *testing.T) {
	if os.Getenv(projectionCrashFlag) != "1" {
		return
	}
	var command ingestionapplication.ProjectDocumentCommand
	if err := json.Unmarshal([]byte(os.Getenv(projectionCrashCommandFlag)), &command); err != nil {
		t.Fatal(err)
	}
	runtime, err := database.Open(context.Background(), os.Getenv(projectionCrashDSNFlag))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	documentVersions, err := ingestionapplication.NewDocumentVersionService(ingestionapplication.DocumentVersionDependencies{
		Observations: &integrationDocumentObservationReader{observations: map[int64]ingestionapplication.DocumentObservationDTO{}},
		Versions:     ingestionpostgres.NewDocumentVersionRepository(runtime),
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := killAfterProjectionPublish{inner: newKnowledgeProjectionPublisher(t, os.Getenv(projectionCrashVaultFlag))}
	saga := newDerivedArtifactSaga(t, runtime, publisher, documentVersions)
	worker := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindRunRetention: func(ctx context.Context, _ queue.Job) error {
		_, err := saga.Project(ctx, command)
		return err
	}})
	if worked, err := worker.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("RunOnce(crash helper) = %t/%v", worked, err)
	}
	t.Fatal("crash helper returned without terminating")
}

type killAfterProjectionPublish struct {
	inner knowledgeapplication.ProjectionPublisher
}

func (publisher killAfterProjectionPublish) Publish(ctx context.Context, command knowledgeapplication.PublishProjectionCommand) (knowledgeapplication.PublishProjectionResult, error) {
	result, err := publisher.inner.Publish(ctx, command)
	if err != nil {
		return result, err
	}
	os.Exit(projectionCrashExitCode)
	return result, nil
}

func assertProjectionCrashFacts(t *testing.T, runtime *database.Runtime, documentVersionID, jobID int64, wantArtifactState string, wantActive bool, wantJobState string, wantAttempt int) {
	t.Helper()
	var artifactState, jobState, relativePath string
	var active bool
	var attempt int
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state,active,vault_relative_path FROM derived_artifacts WHERE document_version_id=$1`, documentVersionID).Scan(&artifactState, &active, &relativePath); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT state,attempt FROM river_job WHERE id=$1`, jobID).Scan(&jobState, &attempt); err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(relativePath) || artifactState != wantArtifactState || active != wantActive || jobState != wantJobState || attempt != wantAttempt {
		t.Fatalf("projection recovery facts = artifact %q active=%t path=%q owner %q/%d",
			artifactState, active, relativePath, jobState, attempt)
	}
}
