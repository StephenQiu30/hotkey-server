package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestGovernanceRepositoryRecordsManualSearchesWithoutProductLimit(t *testing.T) {
	ctx := context.Background()
	runtime := governanceRuntime(t)
	defer runtime.Close()
	userID := governanceUser(t, runtime)
	repository := operationspostgres.NewGovernanceRepository(runtime)
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	var wait sync.WaitGroup
	for index := 0; index < 25; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := repository.RecordManualSearch(ctx, userID, now); err != nil {
				t.Errorf("RecordManualSearch() error = %v", err)
			}
		}()
	}
	wait.Wait()
	overview, err := repository.UsageOverview(ctx, userID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Items) != 6 {
		t.Fatalf("usage item count = %d, want six projections for five product dimensions", len(overview.Items))
	}
	manual := usageByDimension(t, overview, operationsdomain.DimensionManualSearches)
	if manual.Used != "25" || manual.Mode != "observed" || manual.Limit != nil || manual.Remaining != nil || manual.ResetAt == nil || !manual.ResetAt.Equal(time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("manual usage = %#v", manual)
	}
	for index := int64(0); index < operationsdomain.ActiveMonitorLimit; index++ {
		if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO monitors (name,status) VALUES ($1,'active')`, fmt.Sprintf("quota-monitor-%d", index)); err != nil {
			t.Fatal(err)
		}
	}
	var appError *sharederrors.AppError
	if err := repository.CheckActiveMonitor(ctx, 0); !errors.As(err, &appError) || appError.Code != sharederrors.CodeProductQuotaExceeded {
		t.Fatalf("CheckActiveMonitor() error = %#v, want product quota", err)
	}
}

func TestGovernanceAuditCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert(t *testing.T) {
	ctx := context.Background()
	runtime := governanceRuntime(t)
	defer runtime.Close()
	userID := governanceUser(t, runtime)
	ids := []int64{
		insertGovernanceAudit(t, runtime, userID, "monitor.created", "success", 1),
		insertGovernanceAudit(t, runtime, userID, "monitor.published", "failure", 2),
		insertGovernanceAudit(t, runtime, userID, "monitor.published", "success", 3),
	}
	codec, err := pagination.NewCodec("operations-audit-cursor-test-secret-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	repository := operationspostgres.NewGovernanceRepositoryWithCursorCodec(runtime, codec)
	query := operationsdomain.AuditQuery{SubjectUserID: userID, Limit: 2}
	first, err := repository.ListAudit(ctx, query)
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" || first.Items[0].ID != ids[2] || first.Items[1].ID != ids[1] {
		t.Fatalf("first audit page = %#v/%v", first, err)
	}
	if _, err := strconv.ParseInt(first.NextCursor, 10, 64); err == nil || !strings.Contains(first.NextCursor, ".") {
		t.Fatalf("audit cursor is not opaque and signed: %q", first.NextCursor)
	}
	concurrentID := insertGovernanceAudit(t, runtime, userID, "monitor.published", "success", 4)
	query.Cursor = first.NextCursor
	second, err := repository.ListAudit(ctx, query)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != ids[0] || second.Items[0].ID == concurrentID || second.NextCursor != "" {
		t.Fatalf("second audit page = %#v/%v", second, err)
	}
	filtered, err := repository.ListAudit(ctx, operationsdomain.AuditQuery{SubjectUserID: userID, Limit: 10, Action: "monitor.published", Result: "success"})
	if err != nil || len(filtered.Items) != 2 || filtered.Items[0].Action != "monitor.published" || filtered.Items[0].Result != "success" {
		t.Fatalf("filtered audit page = %#v/%v", filtered, err)
	}

	tampered := "A" + first.NextCursor[1:]
	if tampered == first.NextCursor {
		tampered = "B" + first.NextCursor[1:]
	}
	for name, changed := range map[string]operationsdomain.AuditQuery{
		"tampered":     {SubjectUserID: userID, Limit: 2, Cursor: tampered},
		"cross filter": {SubjectUserID: userID, Limit: 2, Action: "monitor.published", Cursor: first.NextCursor},
		"cross subject": {SubjectUserID: userID + 1, Limit: 2,
			Cursor: first.NextCursor},
	} {
		if _, err := repository.ListAudit(ctx, changed); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("%s cursor error = %v, want invalid input", name, err)
		}
	}

	expiringCodec, err := pagination.NewCodec("expiring-audit-cursor-test-secret-32-bytes", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	expiring := operationspostgres.NewGovernanceRepositoryWithCursorCodec(runtime, expiringCodec)
	expiringQuery := operationsdomain.AuditQuery{SubjectUserID: userID, Limit: 1}
	expiringFirst, err := expiring.ListAudit(ctx, expiringQuery)
	if err != nil || expiringFirst.NextCursor == "" {
		t.Fatalf("expiring first page = %#v/%v", expiringFirst, err)
	}
	time.Sleep(5 * time.Millisecond)
	expiringQuery.Cursor = expiringFirst.NextCursor
	if _, err := expiring.ListAudit(ctx, expiringQuery); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired cursor error = %v, want invalid input", err)
	}
}

func TestGovernanceAuditQueryCoversFiveSourceManagementCategoriesWithoutSyntheticSecrets(t *testing.T) {
	ctx := context.Background()
	runtime := governanceRuntime(t)
	defer runtime.Close()
	userID := governanceUser(t, runtime)
	writer := operationspostgres.NewAuditWriter(runtime)
	entries := []operationsdomain.AuditEntry{
		{ActorType: "user", ActorID: userID, Action: operationsdomain.ActionSourceUpdated, ResourceType: "source_connection", ResourceID: 11, Result: operationsdomain.AuditResultSuccess},
		{ActorType: "user", ActorID: userID, Action: operationsdomain.ActionSourceCredentialChanged, ResourceType: "source_connection", ResourceID: 11, Result: operationsdomain.AuditResultSuccess},
		{ActorType: "user", ActorID: userID, Action: operationsdomain.ActionSourceBudgetUpdated, ResourceType: "source_connection", ResourceID: 11, Result: operationsdomain.AuditResultSuccess},
		{ActorType: "user", ActorID: userID, Action: operationsdomain.ActionCollectionManualRequested, ResourceType: "monitor", ResourceID: 12, Result: operationsdomain.AuditResultSuccess},
		{ActorType: "user", ActorID: userID, Action: operationsdomain.ActionRetentionExecuted, ResourceType: "retention_policy", ResourceID: 13, Result: operationsdomain.AuditResultSuccess},
	}
	if err := runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		for _, entry := range entries {
			if err := writer.Write(transactionCtx, entry); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("write five-category management audits: %v", err)
	}

	syntheticSecret := "synthetic-source-token-never-persist"
	leaky := entries[1]
	leaky.ResourceID = 99
	leaky.After = map[string]any{"credential_ref": syntheticSecret}
	if err := runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		return writer.Write(transactionCtx, leaky)
	}); err == nil {
		t.Fatal("synthetic credential was accepted by the audit boundary")
	}

	page, err := operationspostgres.NewGovernanceRepository(runtime).ListAudit(ctx, operationsdomain.AuditQuery{SubjectUserID: userID, Limit: 20})
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	want := map[string]bool{
		string(operationsdomain.ActionSourceUpdated):             false,
		string(operationsdomain.ActionSourceCredentialChanged):   false,
		string(operationsdomain.ActionSourceBudgetUpdated):       false,
		string(operationsdomain.ActionCollectionManualRequested): false,
		string(operationsdomain.ActionRetentionExecuted):         false,
	}
	for _, item := range page.Items {
		if _, found := want[item.Action]; found {
			want[item.Action] = true
		}
	}
	for action, found := range want {
		if !found {
			t.Errorf("unified audit query omitted management action %q", action)
		}
	}
	var persisted string
	if err := runtime.SQL.QueryRow(`SELECT coalesce(string_agg(action || coalesce(before_data::text, '') || coalesce(after_data::text, ''), ''), '') FROM audit_logs`).Scan(&persisted); err != nil {
		t.Fatalf("read audit leak sentinel surface: %v", err)
	}
	if strings.Contains(persisted, syntheticSecret) {
		t.Fatalf("audit surface leaked synthetic secret: %s", persisted)
	}
}

func insertGovernanceAudit(t *testing.T, runtime *database.Runtime, userID int64, action, result string, resourceID int) int64 {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`INSERT INTO audit_logs (actor_type,actor_id,action,resource_type,resource_id,result,before_data,after_data,ip_hash)
VALUES ('user',$1,$2,'monitor',$3,$4,'{"status":"draft"}','{"status":"active"}',$5) RETURNING id`, userID, action, resourceID, result, strings.Repeat("a", 64)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRetentionRunRequiresFrozenPreviewApprovalAndStopsOnCandidateDrift(t *testing.T) {
	ctx := context.Background()
	runtime := governanceRuntime(t)
	defer runtime.Close()
	userID := governanceUser(t, runtime)
	approverID := governanceUser(t, runtime)
	approver := identitydomain.Subject{UserID: approverID, Role: identitydomain.RoleAdmin}
	retention := operationspostgres.NewRetentionRepository(runtime)
	store := operationspostgres.NewGovernanceRepository(runtime)
	service, err := operationsapplication.NewGovernanceService(operationsapplication.GovernanceDependencies{
		Runtime: runtime, Store: store, Retention: retention, Audit: operationspostgres.NewAuditWriter(runtime),
		Now: func() time.Time { return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	policies, err := service.RetentionPolicies(ctx, identitydomain.Subject{UserID: userID, Role: identitydomain.RoleAdmin})
	if err != nil || len(policies) != 7 {
		t.Fatalf("policies = %d/%v, want 7", len(policies), err)
	}
	metricPolicy := retentionPolicy(t, policies, "content_metric_snapshots")
	auditPolicy := retentionPolicy(t, policies, "audit_logs")
	deliveryPolicy := retentionPolicy(t, policies, "delivery_attempts")
	if !auditPolicy.Protected || auditPolicy.Enabled {
		t.Fatalf("audit policy = %#v", auditPolicy)
	}
	if !deliveryPolicy.Protected || deliveryPolicy.Enabled {
		t.Fatalf("delivery attempt policy = %#v, want protected and disabled", deliveryPolicy)
	}
	sourceID := governanceSource(t, runtime)
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		var contentID int64
		if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO contents (source_connection_id,external_id,content_type,canonical_url,published_at,fetched_at,dedupe_key) VALUES ($1,$2,'article',$3,$4,$4,$5) RETURNING id`, sourceID, "retention-"+string(rune('a'+index)), "https://example.test/"+string(rune('a'+index)), old, hashCharacter(index)).Scan(&contentID); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO content_metric_snapshots (content_id,captured_at) VALUES ($1,$2)`, contentID, old); err != nil {
			t.Fatal(err)
		}
	}
	input := operationsapplication.RetentionInput{Subject: identitydomain.Subject{UserID: userID, Role: identitydomain.RoleAdmin}, PolicyID: metricPolicy.ID, ExpectedVersion: metricPolicy.Version, BatchSize: 2}
	if _, err := service.ApproveRetention(ctx, operationsapplication.RetentionRunInput{Subject: input.Subject, RunID: 999999, CandidateHash: strings.Repeat("a", 64)}); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("approve missing run error = %v, want not found", err)
	}
	preview, err := service.PreviewRetention(ctx, input)
	if err != nil || preview.RunID <= 0 || preview.PolicyVersion != metricPolicy.Version || len(preview.CandidateHash) != 64 || preview.Status != "pending_approval" || preview.Affected != 2 || !preview.HasMore || !preview.DryRun {
		t.Fatalf("preview = %#v/%v", preview, err)
	}
	repeatedPreview, err := service.PreviewRetention(ctx, input)
	if err != nil || repeatedPreview.RunID == preview.RunID || repeatedPreview.CandidateHash != preview.CandidateHash || repeatedPreview.Affected != preview.Affected || repeatedPreview.HasMore != preview.HasMore {
		t.Fatalf("repeated preview = %#v/%v, want a new run with the same frozen candidate hash", repeatedPreview, err)
	}
	loaded, err := service.RetentionRun(ctx, approver, preview.RunID)
	if err != nil || loaded.RunID != preview.RunID || loaded.CandidateHash != preview.CandidateHash || loaded.RequestedByUserID != userID || loaded.ApprovedByUserID != 0 || loaded.Status != operationsdomain.RetentionRunPendingApproval {
		t.Fatalf("loaded retention handoff = %#v/%v", loaded, err)
	}
	if _, err := service.ExecuteRetention(ctx, operationsapplication.RetentionRunInput{Subject: input.Subject, RunID: preview.RunID, CandidateHash: preview.CandidateHash}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("execute before approval error = %v, want conflict", err)
	}
	if _, err := service.ApproveRetention(ctx, operationsapplication.RetentionRunInput{Subject: input.Subject, RunID: preview.RunID, CandidateHash: preview.CandidateHash}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("self approval error = %v, want independent approval conflict", err)
	}
	if _, err := service.ApproveRetention(ctx, operationsapplication.RetentionRunInput{Subject: approver, RunID: preview.RunID, CandidateHash: strings.Repeat("f", 64)}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("approve wrong hash error = %v, want conflict", err)
	}
	approved, err := service.ApproveRetention(ctx, operationsapplication.RetentionRunInput{Subject: approver, RunID: preview.RunID, CandidateHash: preview.CandidateHash})
	if err != nil || approved.Status != "approved" || approved.RunID != preview.RunID || approved.RequestedByUserID != userID || approved.ApprovedByUserID != approverID {
		t.Fatalf("approved = %#v/%v", approved, err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE retention_runs SET status='approved',approved_by_user_id=requested_by_user_id,approved_at=$2,updated_at=$2 WHERE id=$1`, repeatedPreview.RunID, time.Date(2026, 8, 8, 12, 0, 1, 0, time.UTC)); !postgresConstraint(err, "retention_runs_independent_approval") {
		t.Fatalf("direct self approval error = %#v, want independent approval constraint", err)
	}
	var before int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM content_metric_snapshots`).Scan(&before); err != nil || before != 3 {
		t.Fatalf("before = %d/%v", before, err)
	}
	var driftedCandidateID int64
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT candidate_id FROM retention_run_items WHERE retention_run_id=$1 ORDER BY ordinal LIMIT 1`, preview.RunID).Scan(&driftedCandidateID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `DELETE FROM content_metric_snapshots WHERE id=$1`, driftedCandidateID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExecuteRetention(ctx, operationsapplication.RetentionRunInput{Subject: input.Subject, RunID: preview.RunID, CandidateHash: preview.CandidateHash}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("execute drifted run error = %v, want conflict", err)
	}
	var blockedStatus, failureCode string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT status,failure_code FROM retention_runs WHERE id=$1`, preview.RunID).Scan(&blockedStatus, &failureCode); err != nil || blockedStatus != "blocked" || failureCode != "candidate_drift" {
		t.Fatalf("blocked run = %q/%q/%v", blockedStatus, failureCode, err)
	}
	var remainingAfterDrift int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM content_metric_snapshots`).Scan(&remainingAfterDrift); err != nil || remainingAfterDrift != 2 {
		t.Fatalf("remaining after drift stop = %d/%v, want 2", remainingAfterDrift, err)
	}

	preview, err = service.PreviewRetention(ctx, input)
	if err != nil || preview.Affected != 2 || preview.HasMore {
		t.Fatalf("second preview = %#v/%v", preview, err)
	}
	if _, err := service.ApproveRetention(ctx, operationsapplication.RetentionRunInput{Subject: approver, RunID: preview.RunID, CandidateHash: preview.CandidateHash}); err != nil {
		t.Fatal(err)
	}
	run, err := service.ExecuteRetention(ctx, operationsapplication.RetentionRunInput{Subject: input.Subject, RunID: preview.RunID, CandidateHash: preview.CandidateHash})
	if err != nil || run.Affected != 2 || run.HasMore || run.DryRun || run.Status != "completed" {
		t.Fatalf("run = %#v/%v", run, err)
	}
	if _, err := service.ExecuteRetention(ctx, operationsapplication.RetentionRunInput{Subject: input.Subject, RunID: preview.RunID, CandidateHash: preview.CandidateHash}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("repeat completed run error = %v, want idempotent conflict", err)
	}
	var remaining int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM content_metric_snapshots`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("remaining = %d/%v", remaining, err)
	}
	var retainedContentID int64
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT id FROM contents ORDER BY id LIMIT 1`).Scan(&retainedContentID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO content_metric_snapshots (content_id,captured_at) VALUES ($1,$2)`, retainedContentID, old); err != nil {
		t.Fatal(err)
	}
	policyDriftPreview, err := service.PreviewRetention(ctx, operationsapplication.RetentionInput{Subject: input.Subject, PolicyID: metricPolicy.ID, ExpectedVersion: metricPolicy.Version, BatchSize: 2})
	if err != nil || policyDriftPreview.Affected != 1 {
		t.Fatalf("policy drift preview = %#v/%v", policyDriftPreview, err)
	}
	if _, err := service.ApproveRetention(ctx, operationsapplication.RetentionRunInput{Subject: approver, RunID: policyDriftPreview.RunID, CandidateHash: policyDriftPreview.CandidateHash}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE retention_policies SET version=version+1 WHERE id=$1`, metricPolicy.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExecuteRetention(ctx, operationsapplication.RetentionRunInput{Subject: input.Subject, RunID: policyDriftPreview.RunID, CandidateHash: policyDriftPreview.CandidateHash}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("execute policy-drifted run error = %v, want conflict", err)
	}
	var policyDriftStatus, policyDriftCode string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT status,failure_code FROM retention_runs WHERE id=$1`, policyDriftPreview.RunID).Scan(&policyDriftStatus, &policyDriftCode); err != nil || policyDriftStatus != "blocked" || policyDriftCode != "policy_drift" {
		t.Fatalf("policy drift run = %q/%q/%v", policyDriftStatus, policyDriftCode, err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM content_metric_snapshots`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("remaining after policy drift stop = %d/%v, want 1", remaining, err)
	}
	var audits int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE action IN ('retention.previewed','retention.approved','retention.blocked','retention.executed') AND resource_type='retention_run'`).Scan(&audits); err != nil || audits != 10 {
		t.Fatalf("retention run audits = %d/%v, want 10", audits, err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE retention_run_items SET candidate_id=candidate_id+1000 WHERE retention_run_id=$1`, preview.RunID); !postgresConstraint(err, "retention_run_items_append_only") {
		t.Fatalf("mutate frozen retention item error = %#v, want append-only rejection", err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `DELETE FROM retention_runs WHERE id=$1`, preview.RunID); !postgresConstraint(err, "retention_runs_retained") {
		t.Fatalf("delete retention run error = %#v, want retained rejection", err)
	}
	protectedInput := input
	protectedInput.PolicyID, protectedInput.ExpectedVersion = auditPolicy.ID, auditPolicy.Version
	if _, err := service.PreviewRetention(ctx, protectedInput); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("protected preview error = %v", err)
	}
	protectedInput.PolicyID, protectedInput.ExpectedVersion = deliveryPolicy.ID, deliveryPolicy.Version
	if _, err := service.PreviewRetention(ctx, protectedInput); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("delivery attempt preview error = %v", err)
	}
	editor := identitydomain.Subject{UserID: userID, Role: identitydomain.RoleEditor}
	if _, err := service.RetentionPolicies(ctx, editor); err == nil {
		t.Fatal("editor retention error = nil")
	}
}

func TestRetentionRunReentersSameApprovedRunAfterTransientDeleteFailure(t *testing.T) {
	ctx := context.Background()
	runtime := governanceRuntime(t)
	defer runtime.Close()
	requesterID := governanceUser(t, runtime)
	approverID := governanceUser(t, runtime)
	requester := identitydomain.Subject{UserID: requesterID, Role: identitydomain.RoleAdmin}
	approver := identitydomain.Subject{UserID: approverID, Role: identitydomain.RoleAdmin}
	service, err := operationsapplication.NewGovernanceService(operationsapplication.GovernanceDependencies{
		Runtime: runtime, Store: operationspostgres.NewGovernanceRepository(runtime), Retention: operationspostgres.NewRetentionRepository(runtime), Audit: operationspostgres.NewAuditWriter(runtime),
		Now: func() time.Time { return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	policies, err := service.RetentionPolicies(ctx, requester)
	if err != nil {
		t.Fatal(err)
	}
	policy := retentionPolicy(t, policies, "content_metric_snapshots")
	sourceID := governanceSource(t, runtime)
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var contentID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO contents (source_connection_id,external_id,content_type,canonical_url,published_at,fetched_at,dedupe_key) VALUES ($1,'retention-reentry','article','https://example.test/retention-reentry',$2,$2,$3) RETURNING id`, sourceID, old, strings.Repeat("d", 64)).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO content_metric_snapshots (content_id,captured_at) VALUES ($1,$2)`, contentID, old); err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewRetention(ctx, operationsapplication.RetentionInput{Subject: requester, PolicyID: policy.ID, ExpectedVersion: policy.Version, BatchSize: 100})
	if err != nil || preview.Affected != 1 {
		t.Fatalf("preview = %#v/%v", preview, err)
	}
	approved, err := service.ApproveRetention(ctx, operationsapplication.RetentionRunInput{Subject: approver, RunID: preview.RunID, CandidateHash: preview.CandidateHash})
	if err != nil || approved.Status != operationsdomain.RetentionRunApproved {
		t.Fatalf("approved = %#v/%v", approved, err)
	}
	const faultSQL = `
CREATE FUNCTION retention_delete_fault_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE='57014', MESSAGE='retention delete fault fixture';
END;
$$;
CREATE TRIGGER retention_delete_fault_fixture
BEFORE DELETE ON content_metric_snapshots
FOR EACH STATEMENT EXECUTE FUNCTION retention_delete_fault_fixture();`
	if _, err := runtime.SQL.ExecContext(ctx, faultSQL); err != nil {
		t.Fatal(err)
	}
	cleanupFault := func() {
		_, _ = runtime.SQL.ExecContext(context.Background(), `DROP TRIGGER IF EXISTS retention_delete_fault_fixture ON content_metric_snapshots; DROP FUNCTION IF EXISTS retention_delete_fault_fixture()`)
	}
	defer cleanupFault()
	input := operationsapplication.RetentionRunInput{Subject: requester, RunID: preview.RunID, CandidateHash: preview.CandidateHash}
	if _, err := service.ExecuteRetention(ctx, input); !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("faulted execution error = %v, want unavailable", err)
	}
	cleanupFault()
	loaded, err := service.RetentionRun(ctx, approver, preview.RunID)
	if err != nil || loaded.RunID != preview.RunID || loaded.Status != operationsdomain.RetentionRunApproved || loaded.CandidateHash != preview.CandidateHash || loaded.Affected != 1 {
		t.Fatalf("reenterable run = %#v/%v", loaded, err)
	}
	var remaining, failedAudits int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM content_metric_snapshots`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("remaining after transient failure = %d/%v, want 1", remaining, err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE action='retention.executed' AND resource_type='retention_run' AND resource_id=$1 AND result='failure' AND after_data->>'reason_code'='execution_failed'`, preview.RunID).Scan(&failedAudits); err != nil || failedAudits != 1 {
		t.Fatalf("failed execution audits = %d/%v, want one sanitized failure", failedAudits, err)
	}
	completed, err := service.ExecuteRetention(ctx, input)
	if err != nil || completed.RunID != preview.RunID || completed.Status != operationsdomain.RetentionRunCompleted || completed.Affected != 1 {
		t.Fatalf("reentered execution = %#v/%v", completed, err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM content_metric_snapshots`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("remaining after reentered execution = %d/%v, want 0", remaining, err)
	}
}

func governanceRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	runtime, err := database.Open(context.Background(), postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.InitializeEmpty(context.Background(), runtime.Pool); err != nil {
		runtime.Close()
		t.Fatal(err)
	}
	return runtime
}

func governanceUser(t *testing.T, runtime *database.Runtime) int64 {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role) VALUES ('governance-' || md5(random()::text) || '@example.test','hash','Governance','admin') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func governanceSource(t *testing.T, runtime *database.Runtime) int64 {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type,name,endpoint) VALUES ('rss','governance-' || md5(random()::text),'https://example.test/feed') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func retentionPolicy(t *testing.T, policies []operationsdomain.RetentionPolicy, dataClass string) operationsdomain.RetentionPolicy {
	t.Helper()
	for _, policy := range policies {
		if policy.DataClass == dataClass {
			return policy
		}
	}
	t.Fatalf("missing retention policy %q", dataClass)
	return operationsdomain.RetentionPolicy{}
}

func usageByDimension(t *testing.T, overview operationsdomain.UsageOverview, dimension string) operationsdomain.UsageItem {
	t.Helper()
	for _, item := range overview.Items {
		if item.Dimension == dimension {
			return item
		}
	}
	t.Fatalf("missing usage %q", dimension)
	return operationsdomain.UsageItem{}
}

func hashCharacter(index int) string { return strings.Repeat(string(rune('a'+index)), 64) }

func postgresConstraint(err error, name string) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23514" && postgresError.ConstraintName == name
}
