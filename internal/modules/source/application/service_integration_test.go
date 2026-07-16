package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestSourceServiceAdminLifecycleAndSafeReads(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := seedAdmin(t, runtime)
	service := newService(t, runtime, usageReader{})
	ctx := context.Background()

	created, err := service.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: sourceConnection("service-lifecycle")})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Version != 1 || created.HealthStatus != domain.HealthStatusUnknown || !created.Enabled {
		t.Fatalf("created source = %#v, want version one, enabled and unknown health", created)
	}
	if created.Config.ContentRetentionDays != 30 || created.Config.MaxPagesPerRun != 1 {
		t.Fatalf("created defaults = %#v, want stable P0 defaults", created.Config)
	}

	public, err := service.GetPublic(ctx, identitydomain.Subject{UserID: 2, Role: identitydomain.RoleViewer}, created.ID)
	if err != nil {
		t.Fatalf("GetPublic() error = %v", err)
	}
	if public.CredentialConfigured || public.Deleted || public.Name != created.Name {
		t.Fatalf("public source = %#v, want safe active projection", public)
	}
	monitorSource, err := service.FindForMonitor(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindForMonitor() error = %v", err)
	}
	for _, value := range []any{public, *created, monitorSource} {
		if _, found := reflect.TypeOf(value).FieldByName("CredentialRef"); found {
			t.Fatalf("%T exposes credential reference", value)
		}
	}
	if _, err := service.Create(ctx, sourceapplication.CreateInput{Subject: identitydomain.Subject{UserID: 3, Role: identitydomain.RoleEditor}, Connection: sourceConnection("editor-denied")}); appCode(err) != sharederrors.CodeForbidden {
		t.Fatalf("editor Create() code = %d, want forbidden", appCode(err))
	}
	if _, err := service.Update(ctx, sourceapplication.UpdateInput{Subject: admin, ID: created.ID, ExpectedVersion: 99}); appCode(err) != sharederrors.CodeSourceConnectionUnavailable {
		t.Fatalf("stale Update() code = %d, want source unavailable", appCode(err))
	}
	updatedName := "service-lifecycle-renamed"
	updated, err := service.Update(ctx, sourceapplication.UpdateInput{Subject: admin, ID: created.ID, ExpectedVersion: created.Version, Name: &updatedName})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != updatedName || updated.Version != created.Version+1 {
		t.Fatalf("updated source = %#v, want renamed source with advanced version", updated)
	}
	for _, connection := range []domain.SourceConnection{
		{SourceType: domain.SourceType("mastodon"), Name: "bad type", Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true},
		{SourceType: domain.SourceTypeRSS, Name: "bad endpoint", Endpoint: "http://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true},
	} {
		_, err := service.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: connection})
		if err == nil {
			t.Fatalf("invalid Create(%#v) = nil error", connection)
		}
	}

	disabled, err := service.Disable(ctx, sourceapplication.LifecycleInput{Subject: admin, ID: created.ID, ExpectedVersion: updated.Version})
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if disabled.Enabled || disabled.Version != updated.Version+1 {
		t.Fatalf("disabled source = %#v, want disabled source with advanced version", disabled)
	}
	enabled, err := service.Enable(ctx, sourceapplication.LifecycleInput{Subject: admin, ID: created.ID, ExpectedVersion: disabled.Version})
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	archived, err := service.Archive(ctx, sourceapplication.LifecycleInput{Subject: admin, ID: created.ID, ExpectedVersion: enabled.Version})
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if !archived.Deleted || archived.Enabled {
		t.Fatalf("archived source = %#v, want deleted and disabled", archived)
	}
	restored, err := service.Restore(ctx, sourceapplication.LifecycleInput{Subject: admin, ID: created.ID, ExpectedVersion: archived.Version})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if restored.Deleted || restored.Enabled || restored.HealthStatus != domain.HealthStatusUnknown {
		t.Fatalf("restored source = %#v, want non-deleted disabled unknown source", restored)
	}
}

func TestSourceServiceUsageAndAuditFailureRollback(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := seedAdmin(t, runtime)
	ctx := context.Background()
	service := newService(t, runtime, usageReader{usage: domain.SourceUsage{SoleSchedulableForActive: true}})
	created, err := service.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: sourceConnection("usage-protected")})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := service.Disable(ctx, sourceapplication.LifecycleInput{Subject: admin, ID: created.ID, ExpectedVersion: created.Version}); appCode(err) != sharederrors.CodeSourceConnectionRequired {
		t.Fatalf("active sole source Disable() code = %d, want source required", appCode(err))
	}
	pausedUsageService := newService(t, runtime, usageReader{usage: domain.SourceUsage{ReferencedByPausedMonitor: true, PausedMonitorCount: 1}})
	pausedSource, err := pausedUsageService.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: sourceConnection("paused-usage")})
	if err != nil {
		t.Fatalf("Create(paused usage) error = %v", err)
	}
	if _, err := pausedUsageService.Disable(ctx, sourceapplication.LifecycleInput{Subject: admin, ID: pausedSource.ID, ExpectedVersion: pausedSource.Version}); err != nil {
		t.Fatalf("Disable(paused usage) error = %v, want historical paused usage to remain valid", err)
	}

	failing, err := sourceapplication.NewService(sourceapplication.Dependencies{Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), MonitorUsage: usageReader{}, Audit: failingAudit{err: errors.New("audit unavailable")}})
	if err != nil {
		t.Fatalf("NewService(failing audit) error = %v", err)
	}
	if _, err := failing.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: sourceConnection("audit-rollback")}); err == nil {
		t.Fatal("Create() with failing audit = nil error, want rollback")
	}
	var count int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM source_connections WHERE name = 'audit-rollback'`).Scan(&count); err != nil {
		t.Fatalf("count audit rollback source: %v", err)
	}
	if count != 0 {
		t.Fatalf("audit-failed source count = %d, want 0", count)
	}
}

func TestSourceServiceListsSafeCursorPagesAndNormalizesCreateState(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	admin := seedAdmin(t, runtime)
	service := newService(t, runtime, usageReader{})
	ctx := context.Background()

	for index := 1; index <= 3; index++ {
		connection := sourceConnection(fmt.Sprintf("list-source-%d", index))
		if index == 1 {
			connection.AuthType = domain.AuthTypeBearer
			connection.CredentialRef = "env:LIST_SOURCE_TOKEN"
		}
		if _, err := service.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: connection}); err != nil {
			t.Fatalf("Create(list source %d) error = %v", index, err)
		}
	}
	createdDeleted := sourceConnection("create-deleted-normalized")
	createdDeleted.Deleted = true
	normalized, err := service.Create(ctx, sourceapplication.CreateInput{Subject: admin, Connection: createdDeleted})
	if err != nil {
		t.Fatalf("Create(deleted input) error = %v", err)
	}
	if normalized.Deleted {
		t.Fatal("Create(deleted input) returned archived state, want forced active lifecycle state")
	}
	var deletedAt any
	var auditDeleted string
	if err := runtime.SQL.QueryRow(`SELECT deleted_at, after_data->>'deleted' FROM source_connections JOIN audit_logs ON audit_logs.resource_id = source_connections.id WHERE source_connections.id = $1 AND audit_logs.action = 'source.created'`, normalized.ID).Scan(&deletedAt, &auditDeleted); err != nil {
		t.Fatalf("read normalized create state and audit: %v", err)
	}
	if deletedAt != nil || auditDeleted != "false" {
		t.Fatalf("normalized create DB/audit state = deleted_at=%v audit=%q, want NULL/false", deletedAt, auditDeleted)
	}

	viewer := identitydomain.Subject{UserID: admin.UserID, SessionID: 2, Role: identitydomain.RoleViewer}
	first, err := service.ListPublic(ctx, sourceapplication.ListInput{Subject: viewer, Query: domain.SourceConnectionListQuery{Limit: 2}})
	if err != nil {
		t.Fatalf("ListPublic(first page) error = %v", err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" || first.Items[0].ID >= first.Items[1].ID {
		t.Fatalf("first public page = %#v, want stable two-item id-ascending page with cursor", first)
	}
	if !first.Items[0].CredentialConfigured {
		t.Fatalf("first public item = %#v, want configured credential flag", first.Items[0])
	}
	if _, found := reflect.TypeOf(first.Items[0]).FieldByName("CredentialRef"); found {
		t.Fatal("public list item exposes credential reference")
	}
	second, err := service.ListPublic(ctx, sourceapplication.ListInput{Subject: viewer, Query: domain.SourceConnectionListQuery{Cursor: first.NextCursor, Limit: 2}})
	if err != nil {
		t.Fatalf("ListPublic(second page) error = %v", err)
	}
	if len(second.Items) != 2 || second.Items[0].ID <= first.Items[1].ID || second.NextCursor != "" {
		t.Fatalf("second public page = %#v, want remaining id-ascending items and no cursor", second)
	}
	management, err := service.ListManagement(ctx, sourceapplication.ListInput{Subject: admin, Query: domain.SourceConnectionListQuery{Limit: 1}})
	if err != nil {
		t.Fatalf("ListManagement() error = %v", err)
	}
	if len(management.Items) != 1 || management.Items[0].Endpoint == "" || management.Items[0].Config.RequestTimeoutSeconds != 30 {
		t.Fatalf("management list = %#v, want one safe management item", management)
	}
	if _, err := service.ListManagement(ctx, sourceapplication.ListInput{Subject: viewer, Query: domain.SourceConnectionListQuery{Limit: 1}}); appCode(err) != sharederrors.CodeForbidden {
		t.Fatalf("viewer ListManagement() code = %d, want forbidden", appCode(err))
	}
	if _, err := service.ListPublic(ctx, sourceapplication.ListInput{Subject: identitydomain.Subject{}, Query: domain.SourceConnectionListQuery{Limit: 1}}); appCode(err) != sharederrors.CodeUnauthenticated {
		t.Fatalf("anonymous ListPublic() code = %d, want unauthenticated", appCode(err))
	}
	if _, err := service.ListPublic(ctx, sourceapplication.ListInput{Subject: viewer, Query: domain.SourceConnectionListQuery{Cursor: "bad", Limit: 1}}); appCode(err) != sharederrors.CodeInvalidRequest {
		t.Fatalf("invalid cursor ListPublic() code = %d, want invalid request", appCode(err))
	}
	if _, err := service.ListPublic(ctx, sourceapplication.ListInput{Subject: viewer, Query: domain.SourceConnectionListQuery{Limit: 201}}); appCode(err) != sharederrors.CodeInvalidRequest {
		t.Fatalf("oversized ListPublic() code = %d, want invalid request", appCode(err))
	}
}

func openRuntime(t *testing.T) *database.Runtime {
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

func seedAdmin(t *testing.T, runtime *database.Runtime) identitydomain.Subject {
	t.Helper()
	var id int64
	name := fmt.Sprintf("source-admin-%d@example.test", time.Now().UnixNano())
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email, password_hash, display_name, role, status)
VALUES ($1, 'hash', 'Source Admin', 'admin', 'active') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return identitydomain.Subject{UserID: id, SessionID: 1, Role: identitydomain.RoleAdmin}
}

func newService(t *testing.T, runtime *database.Runtime, usage usageReader) *sourceapplication.Service {
	t.Helper()
	service, err := sourceapplication.NewService(sourceapplication.Dependencies{Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), MonitorUsage: usage, Audit: operationspostgres.NewAuditWriter(runtime)})
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	return service
}

func sourceConnection(name string) domain.SourceConnection {
	return domain.SourceConnection{SourceType: domain.SourceTypeRSS, Name: name, Endpoint: "https://feeds.example.test/rss", AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true}
}

func appCode(err error) int {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError.Code
	}
	return 0
}

type usageReader struct{ usage domain.SourceUsage }

func (reader usageReader) UsageForSource(context.Context, int64) (domain.SourceUsage, error) {
	return reader.usage, nil
}

type failingAudit struct{ err error }

var _ operationsapplication.AuditWriter = failingAudit{}

func (audit failingAudit) Write(context.Context, operationsdomain.AuditEntry) error { return audit.err }
