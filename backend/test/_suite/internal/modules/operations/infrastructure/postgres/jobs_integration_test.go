package postgres_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
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

func TestJobRepositoryCursorIsSignedBoundExpiringAndSnapshotStable(t *testing.T) {
	ctx := context.Background()
	runtime := openOperationsRuntime(t)
	defer func() { _ = runtime.Close() }()
	ids := []int64{
		insertOperationsJob(t, runtime, "cursor-job-1", "normalize_content"),
		insertOperationsJob(t, runtime, "cursor-job-2", "normalize_content"),
		insertOperationsJob(t, runtime, "cursor-job-3", "normalize_content"),
	}
	codec, err := pagination.NewCodec("operations-job-cursor-test-secret-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	repository := operationspostgres.NewJobRepositoryWithCursorCodec(runtime, codec)
	query := operationsdomain.JobListQuery{SubjectUserID: 7, Kind: "normalize_content", State: operationsdomain.JobAvailable, Limit: 2}
	first, err := repository.ListJobs(ctx, query)
	if err != nil || len(first.Items) != 2 || first.Items[0].ID != ids[0] || first.Items[1].ID != ids[1] || first.NextCursor == "" {
		t.Fatalf("first job page = %#v/%v", first, err)
	}
	if _, err := strconv.ParseInt(first.NextCursor, 10, 64); err == nil || !strings.Contains(first.NextCursor, ".") {
		t.Fatalf("job cursor is not opaque and signed: %q", first.NextCursor)
	}
	concurrentID := insertOperationsJob(t, runtime, "cursor-job-concurrent", "normalize_content")
	query.Cursor = first.NextCursor
	second, err := repository.ListJobs(ctx, query)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != ids[2] || second.Items[0].ID == concurrentID || second.NextCursor != "" {
		t.Fatalf("second job page = %#v/%v", second, err)
	}

	tampered := "A" + first.NextCursor[1:]
	if tampered == first.NextCursor {
		tampered = "B" + first.NextCursor[1:]
	}
	for name, changed := range map[string]operationsdomain.JobListQuery{
		"tampered":     {SubjectUserID: 7, Kind: "normalize_content", State: operationsdomain.JobAvailable, Limit: 2, Cursor: tampered},
		"cross filter": {SubjectUserID: 7, Kind: "collect_source", State: operationsdomain.JobAvailable, Limit: 2, Cursor: first.NextCursor},
		"cross subject": {SubjectUserID: 8, Kind: "normalize_content", State: operationsdomain.JobAvailable, Limit: 2,
			Cursor: first.NextCursor},
	} {
		if _, err := repository.ListJobs(ctx, changed); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("%s cursor error = %v, want invalid input", name, err)
		}
	}

	expiringCodec, err := pagination.NewCodec("expiring-job-cursor-test-secret-32-bytes", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	expiring := operationspostgres.NewJobRepositoryWithCursorCodec(runtime, expiringCodec)
	expiringQuery := operationsdomain.JobListQuery{SubjectUserID: 7, Limit: 1}
	expiringFirst, err := expiring.ListJobs(ctx, expiringQuery)
	if err != nil || expiringFirst.NextCursor == "" {
		t.Fatalf("expiring first page = %#v/%v", expiringFirst, err)
	}
	time.Sleep(5 * time.Millisecond)
	expiringQuery.Cursor = expiringFirst.NextCursor
	if _, err := expiring.ListJobs(ctx, expiringQuery); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired cursor error = %v, want invalid input", err)
	}
}

func insertOperationsJob(t *testing.T, runtime *database.Runtime, uniqueKey, kind string) int64 {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO river_job (kind,args,state,attempt,max_attempts,priority,scheduled_at,unique_key)
VALUES ($1,'{"entity_id":1,"entity_version":1}'::jsonb,'available',0,3,1,now(),$2)
RETURNING id`, kind, uniqueKey).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
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
