package postgres_test

import (
	"context"
	"testing"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestJobRepositoryListsCancelsAndRetriesSafeProjection(t *testing.T) {
	runtime := openOperationsRuntime(t)
	defer func() { _ = runtime.Close() }()
	store := queue.NewStore(runtime)
	jobID, created, err := store.Enqueue(context.Background(), queue.Job{
		Kind: queue.KindCollectSource, UniqueKey: "operations-job-integration", Payload: queue.Payload{EntityID: 1, EntityVersion: 1, InputHash: "operations"},
		ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 1,
	})
	if err != nil || !created || jobID <= 0 {
		t.Fatalf("enqueue job = %d/%t/%v", jobID, created, err)
	}
	repository := operationspostgres.NewJobRepository(runtime)
	page, err := repository.ListJobs(context.Background(), operationsdomain.JobListQuery{Limit: 10})
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("ListJobs() = %#v/%v", page, err)
	}
	cancelled, err := repository.CancelJob(context.Background(), jobID)
	if err != nil || cancelled.State != operationsdomain.JobCancelled {
		t.Fatalf("CancelJob() = %#v/%v", cancelled, err)
	}
	retried, err := repository.RetryJob(context.Background(), jobID)
	if err != nil || retried.State != operationsdomain.JobAvailable || retried.Attempt != 0 {
		t.Fatalf("RetryJob() = %#v/%v", retried, err)
	}
}

func openOperationsRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	return runtime
}
