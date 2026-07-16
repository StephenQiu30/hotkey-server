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
