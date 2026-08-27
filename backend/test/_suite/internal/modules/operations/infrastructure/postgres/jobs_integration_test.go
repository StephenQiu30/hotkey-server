package postgres_test

import (
	"context"
	"testing"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestJobRepositoryListsCancelsAndRetriesSafeProjection(t *testing.T) {
	runtime := openOperationsRuntime(t)
	defer func() { _ = runtime.Close() }()
	var actorID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role) VALUES ('job-operator@example.test','hash','Job operator','admin') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	store := queue.NewStore(runtime)
	jobID, created, err := store.Enqueue(context.Background(), queue.Job{
		Kind: queue.KindNormalizeContent, UniqueKey: "operations-job-integration", Payload: queue.Payload{EntityID: 1, EntityVersion: 1, InputHash: "operations"},
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
	service, err := operationsapplication.NewJobService(repository, operationspostgres.NewAuditWriter(runtime))
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.Cancel(context.Background(), operationsdomain.JobMutationInput{ActorID: actorID, JobID: jobID})
	if err != nil || cancelled.State != operationsdomain.JobCancelled {
		t.Fatalf("CancelJob() = %#v/%v", cancelled, err)
	}
	retried, err := service.Retry(context.Background(), operationsdomain.JobMutationInput{ActorID: actorID, JobID: jobID})
	if err != nil || retried.State != operationsdomain.JobAvailable || retried.Attempt != 0 {
		t.Fatalf("RetryJob() = %#v/%v", retried, err)
	}
	var audits int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE resource_type='river_job' AND resource_id=$1 AND action IN ('job.cancelled','job.retried')`, jobID).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("job audit count = %d/%v, want 2", audits, err)
	}
}

func TestJobRepositoryProjectsOnlySafeResourceAndFailureCodes(t *testing.T) {
	ctx := context.Background()
	runtime := openOperationsRuntime(t)
	defer func() { _ = runtime.Close() }()
	var jobID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO river_job (kind,args,state,attempt,max_attempts,priority,scheduled_at,finalized_at,unique_key)
VALUES ('collect_source','{"entity_id":77,"entity_version":1,"query":"private-topic"}','discarded',1,1,1,now(),now(),'safe-job') RETURNING id`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO river_job_attempt (job_id,attempt,error) VALUES ($1,1,'upstream secret')`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO river_job (kind,args,state,attempt,max_attempts,priority,scheduled_at,finalized_at,unique_key)
VALUES ('collect_source','{"entity_id":9999999999999999999,"entity_version":1}','discarded',1,1,1,now(),now(),'overflow-resource-job')`); err != nil {
		t.Fatal(err)
	}
	page, err := operationspostgres.NewJobRepository(runtime).ListJobs(ctx, operationsdomain.JobListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ResourceID != 77 || page.Items[0].FailureCode != "" || page.Items[1].ResourceID != 0 {
		t.Fatalf("unsafe legacy projection = %#v", page.Items)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE river_job_attempt SET error='permanent' WHERE job_id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	page, err = operationspostgres.NewJobRepository(runtime).ListJobs(ctx, operationsdomain.JobListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Items[0].FailureCode != "permanent" {
		t.Fatalf("safe failure code = %q, want permanent", page.Items[0].FailureCode)
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
