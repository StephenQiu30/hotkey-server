package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestAuditWriterUsesCallerTransactionAndPersistsOnlySafeMetadata(t *testing.T) {
	runtime := newOperationsRuntime(t)
	writer := NewAuditWriter(runtime)
	entry := operationsdomain.AuditEntry{
		ActorType:    "user",
		ActorID:      42,
		Action:       operationsdomain.ActionMonitorPublished,
		ResourceType: "monitor",
		ResourceID:   7,
		Result:       operationsdomain.AuditResultSuccess,
		Before:       map[string]any{"status": "draft", "endpoint": "https://private.example.test/feed"},
		After:        map[string]any{"status": "active", "config_hash": strings.Repeat("a", 64), "credential_ref": "env:SECRET"},
	}
	if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		return writer.Write(ctx, entry)
	}); err == nil {
		t.Fatal("Write() accepted sensitive metadata, want rejection")
	}

	entry.Before = map[string]any{"status": "draft", "monitor_version": int64(3)}
	entry.After = map[string]any{"status": "active", "config_hash": strings.Repeat("a", 64), "source_count": int64(2)}
	entry.RequestID = "request-audit-77"
	entry.TraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	entry.IPHash = strings.Repeat("b", 64)
	if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		return writer.Write(ctx, entry)
	}); err != nil {
		t.Fatalf("Write() inside caller transaction: %v", err)
	}

	var before, after string
	if err := runtime.SQL.QueryRow(`SELECT COALESCE(before_data::text, ''), COALESCE(after_data::text, '') FROM audit_logs WHERE action = $1`, string(entry.Action)).Scan(&before, &after); err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	for _, forbidden := range []string{"private.example.test", "env:SECRET", "endpoint", "credential_ref"} {
		if strings.Contains(before, forbidden) || strings.Contains(after, forbidden) {
			t.Fatalf("audit metadata leaked %q: before=%s after=%s", forbidden, before, after)
		}
	}
	if !strings.Contains(after, "config_hash") || !strings.Contains(after, "source_count") {
		t.Fatalf("safe audit metadata missing: after=%s", after)
	}
	if err := writer.Write(context.Background(), entry); !errors.Is(err, ErrTransactionRequired) {
		t.Fatalf("Write() outside transaction error = %v, want ErrTransactionRequired", err)
	}
}

func TestAuditWriterRejectsSensitiveValuesInEveryPermittedStringFieldWithoutPersistence(t *testing.T) {
	runtime := newOperationsRuntime(t)
	writer := NewAuditWriter(runtime)
	stringKeys := []string{"status", "previous_status", "approval_status", "config_hash", "published_at"}
	sensitiveValues := []string{
		"https://private.example.test/feed?token=secret",
		"env:HOTKEY_SOURCE_TOKEN",
		"raw rule phrase that must not be audited",
		`{"config":{"secret":"do-not-persist"}}`,
		"SELECT endpoint, credential_ref FROM source_connections",
	}
	for _, key := range stringKeys {
		for _, value := range sensitiveValues {
			entry := safeAuditEntry()
			entry.After = map[string]any{key: value}
			if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
				return writer.Write(ctx, entry)
			}); err == nil {
				t.Errorf("Write() accepted sensitive value under permitted key %q: %q", key, value)
			}
		}
	}

	var count int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs`).Scan(&count); err != nil {
		t.Fatalf("count rejected audit rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("rejected string metadata wrote %d audit rows, want 0", count)
	}
}

func TestAuditEntryRejectsUnboundedCorrelationAndIdentityStrings(t *testing.T) {
	t.Parallel()

	valid := safeAuditEntry()
	valid.Before = map[string]any{"status": "draft"}
	valid.After = map[string]any{
		"status":          "active",
		"approval_status": "approved",
		"config_hash":     strings.Repeat("a", 64),
		"published_at":    "2026-07-16T08:00:00Z",
	}
	valid.RequestID = "request-audit-77"
	valid.TraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	valid.IPHash = strings.Repeat("b", 64)
	if err := valid.Validate(); err != nil {
		t.Fatalf("safe AuditEntry.Validate() error = %v", err)
	}

	for _, mutate := range []func(*operationsdomain.AuditEntry){
		func(entry *operationsdomain.AuditEntry) { entry.ActorType = "https://private.example.test" },
		func(entry *operationsdomain.AuditEntry) { entry.ResourceType = "source endpoint" },
		func(entry *operationsdomain.AuditEntry) { entry.RequestID = "env:HOTKEY_TOKEN" },
		func(entry *operationsdomain.AuditEntry) { entry.TraceID = "SELECT secret FROM audit_logs" },
		func(entry *operationsdomain.AuditEntry) { entry.IPHash = "127.0.0.1" },
	} {
		entry := valid
		mutate(&entry)
		if err := entry.Validate(); err == nil {
			t.Error("AuditEntry.Validate() accepted an unbounded correlation or identity string")
		}
	}
}

func TestAuditWriterRejectsUnknownActionsAndRollsBackWithCaller(t *testing.T) {
	runtime := newOperationsRuntime(t)
	writer := NewAuditWriter(runtime)
	entry := operationsdomain.AuditEntry{
		ActorType: "user", Action: operationsdomain.AuditAction("monitor.delete_everything"), ResourceType: "monitor", Result: operationsdomain.AuditResultSuccess,
	}
	if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		return writer.Write(ctx, entry)
	}); err == nil {
		t.Fatal("unknown audit action = nil error, want rejection")
	}

	entry.Action = operationsdomain.ActionSourceCreated
	entry.ResourceType = "source_connection"
	if err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, _ database.Transaction) error {
		if err := writer.Write(ctx, entry); err != nil {
			return err
		}
		return errors.New("force caller rollback")
	}); err == nil {
		t.Fatal("rollback callback = nil error, want propagated error")
	}
	var count int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM audit_logs WHERE action = $1`, string(entry.Action)).Scan(&count); err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back audit log count = %d, want 0", count)
	}
}

func TestAuditActionWhitelistIsClosed(t *testing.T) {
	t.Parallel()
	for _, action := range []operationsdomain.AuditAction{
		operationsdomain.ActionMonitorCreated, operationsdomain.ActionMonitorDraftUpdated, operationsdomain.ActionMonitorAICandidateCreated,
		operationsdomain.ActionMonitorAICandidateApproved, operationsdomain.ActionMonitorAICandidateRejected, operationsdomain.ActionMonitorPublished,
		operationsdomain.ActionMonitorPaused, operationsdomain.ActionMonitorResumed, operationsdomain.ActionMonitorArchived, operationsdomain.ActionMonitorRestored,
		operationsdomain.ActionSourceCreated, operationsdomain.ActionSourceUpdated, operationsdomain.ActionSourceEnabled, operationsdomain.ActionSourceDisabled,
		operationsdomain.ActionSourceArchived, operationsdomain.ActionSourceRestored,
	} {
		if !action.Valid() {
			t.Errorf("whitelisted action %q is invalid", action)
		}
	}
	if operationsdomain.AuditAction("monitor.previewed").Valid() {
		t.Fatal("unapproved action was accepted")
	}
}

func safeAuditEntry() operationsdomain.AuditEntry {
	return operationsdomain.AuditEntry{
		ActorType: "user", ActorID: 42, Action: operationsdomain.ActionMonitorPublished,
		ResourceType: "monitor", ResourceID: 7, Result: operationsdomain.AuditResultSuccess,
	}
}

func newOperationsRuntime(t *testing.T) *database.Runtime {
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
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}
