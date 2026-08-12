package postgres_test

import (
	"context"
	"errors"
	"fmt"
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
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
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

func TestGovernanceAuditQueryUsesStableFilteredCursor(t *testing.T) {
	ctx := context.Background()
	runtime := governanceRuntime(t)
	defer runtime.Close()
	userID := governanceUser(t, runtime)
	for index, result := range []string{"success", "failure", "success"} {
		if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO audit_logs (actor_type,actor_id,action,resource_type,resource_id,result,before_data,after_data,ip_hash) VALUES ('user',$1,$2,'monitor',$3,$4,'{"status":"draft"}','{"status":"active"}',$5)`, userID, []string{"monitor.created", "monitor.published", "monitor.published"}[index], index+1, result, strings.Repeat("a", 64)); err != nil {
			t.Fatal(err)
		}
	}
	repository := operationspostgres.NewGovernanceRepository(runtime)
	first, err := repository.ListAudit(ctx, operationsdomain.AuditQuery{Limit: 2})
	if err != nil || len(first.Items) != 2 || first.NextCursor == 0 || first.Items[0].ID <= first.Items[1].ID {
		t.Fatalf("first audit page = %#v/%v", first, err)
	}
	second, err := repository.ListAudit(ctx, operationsdomain.AuditQuery{Limit: 2, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID >= first.NextCursor {
		t.Fatalf("second audit page = %#v/%v", second, err)
	}
	filtered, err := repository.ListAudit(ctx, operationsdomain.AuditQuery{Limit: 10, Action: "monitor.published", Result: "success"})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Action != "monitor.published" || filtered.Items[0].Result != "success" {
		t.Fatalf("filtered audit page = %#v/%v", filtered, err)
	}
}

func TestRetentionPreviewRunIsBoundedProtectedAndAudited(t *testing.T) {
	ctx := context.Background()
	runtime := governanceRuntime(t)
	defer runtime.Close()
	userID := governanceUser(t, runtime)
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
	if !auditPolicy.Protected || auditPolicy.Enabled {
		t.Fatalf("audit policy = %#v", auditPolicy)
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
	preview, err := service.PreviewRetention(ctx, input)
	if err != nil || preview.Affected != 2 || !preview.HasMore || !preview.DryRun {
		t.Fatalf("preview = %#v/%v", preview, err)
	}
	var before int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM content_metric_snapshots`).Scan(&before); err != nil || before != 3 {
		t.Fatalf("before = %d/%v", before, err)
	}
	run, err := service.RunRetention(ctx, input)
	if err != nil || run.Affected != 2 || !run.HasMore || run.DryRun {
		t.Fatalf("run = %#v/%v", run, err)
	}
	var remaining, audits int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM content_metric_snapshots`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("remaining = %d/%v", remaining, err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE action='retention.executed' AND resource_id=$1`, metricPolicy.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits = %d/%v", audits, err)
	}
	protectedInput := input
	protectedInput.PolicyID, protectedInput.ExpectedVersion = auditPolicy.ID, auditPolicy.Version
	if _, err := service.PreviewRetention(ctx, protectedInput); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("protected preview error = %v", err)
	}
	editor := identitydomain.Subject{UserID: userID, Role: identitydomain.RoleEditor}
	if _, err := service.RetentionPolicies(ctx, editor); err == nil {
		t.Fatal("editor retention error = nil")
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
