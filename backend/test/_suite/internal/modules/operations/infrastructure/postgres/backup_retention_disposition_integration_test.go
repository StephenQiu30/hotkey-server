package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestBackupRetentionDispositionIsBoundToSuccessfulBackupAndAppendOnly(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	backupService, err := operationsapplication.NewBackupRunService(operationspostgres.NewBackupRunRepository(runtime))
	if err != nil {
		t.Fatal(err)
	}
	backup, err := backupService.Record(ctx, backupManifest("b", "succeeded", "", time.Now().UTC().Add(-5*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	service, err := operationsapplication.NewBackupRetentionDispositionService(operationspostgres.NewBackupRetentionDispositionRepository(runtime))
	if err != nil {
		t.Fatal(err)
	}
	payload := backupRetentionDispositionManifest("a", "b", "rights_revoked", time.Now().UTC().Add(-time.Minute))
	receipt, err := service.Record(ctx, payload)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := service.Record(ctx, payload)
	if err != nil || repeated != receipt {
		t.Fatalf("idempotent disposition=%+v error=%v", repeated, err)
	}
	var count int64
	var backupRunID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
SELECT count(*),min(backup_run_id) FROM backup_retention_dispositions WHERE backup_run_sha256=$1`, backup.RunSHA256).Scan(&count, &backupRunID); err != nil {
		t.Fatal(err)
	}
	if count != 1 || backupRunID != backup.RunID {
		t.Fatalf("disposition count=%d backup_run_id=%d want=%d", count, backupRunID, backup.RunID)
	}
	if _, err := service.Record(ctx, backupRetentionDispositionManifest("d", "b", "retention_expired", time.Now().UTC().Add(-time.Minute))); err == nil {
		t.Fatal("same backup accepted a conflicting disposition manifest")
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE backup_retention_dispositions SET reason_code='retention_expired' WHERE id=$1`, receipt.DispositionID); err == nil {
		t.Fatal("backup disposition facts must be append-only")
	}
	if _, err := runtime.SQL.ExecContext(ctx, `DELETE FROM backup_retention_dispositions WHERE id=$1`, receipt.DispositionID); err == nil {
		t.Fatal("backup disposition facts must not be deleted")
	}
}

func TestBackupRetentionDispositionRejectsUnknownOrFailedBackupRun(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	backupService, err := operationsapplication.NewBackupRunService(operationspostgres.NewBackupRunRepository(runtime))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backupService.Record(ctx, backupManifest("f", "failed", "backup_timeout", time.Time{})); err != nil {
		t.Fatal(err)
	}
	service, err := operationsapplication.NewBackupRetentionDispositionService(operationspostgres.NewBackupRetentionDispositionRepository(runtime))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"e", "f"} {
		if _, err := service.Record(ctx, backupRetentionDispositionManifest("a", marker, "rights_revoked", time.Now().UTC().Add(-time.Minute))); err == nil {
			t.Fatalf("backup %q accepted a disposition", marker)
		}
	}
	var count int64
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM backup_retention_dispositions`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid dispositions persisted count=%d error=%v", count, err)
	}
}

func backupRetentionDispositionManifest(dispositionMarker, backupMarker, reason string, disposedAt time.Time) []byte {
	return []byte(`{"version":"hotkey-backup-retention-disposition-v1","disposition_sha256":"` + strings.Repeat(dispositionMarker, 64) +
		`","backup_run_sha256":"` + strings.Repeat(backupMarker, 64) + `","deletion_evidence_sha256":"` + strings.Repeat("c", 64) +
		`","reason_code":"` + reason + `","operator_record_id":"backup-operator","reviewer_record_id":"backup-reviewer","disposed_at":"` +
		disposedAt.Format(time.RFC3339Nano) + `"}`)
}
