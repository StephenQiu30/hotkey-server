package postgres_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestBackupRunFactsTriggerFailedAndStaleAlertsThenClear(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository := operationspostgres.NewBackupRunRepository(runtime)
	service, err := operationsapplication.NewBackupRunService(repository)
	if err != nil {
		t.Fatal(err)
	}

	failedPayload := backupManifest("a", "failed", "postgres_backup_failed", time.Time{})
	failed, err := service.Record(ctx, failedPayload)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := service.Record(ctx, failedPayload)
	if err != nil || repeated != failed {
		t.Fatalf("idempotent receipt=%+v err=%v", repeated, err)
	}
	var failedFactCount int64
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM backup_runs WHERE run_sha256=$1`, strings.Repeat("a", 64)).Scan(&failedFactCount); err != nil || failedFactCount != 1 {
		t.Fatalf("idempotent fact count=%d err=%v", failedFactCount, err)
	}
	if _, err := service.Record(ctx, backupManifest("a", "succeeded", "", time.Now().UTC().Add(-time.Minute))); err == nil {
		t.Fatal("same run identity accepted a different manifest")
	}
	assertBackupAlert(t, runtime, failed.RunID, 1, 900)

	stale, err := service.Record(ctx, backupManifest("b", "succeeded", "", time.Now().UTC().Add(-20*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	assertBackupAlert(t, runtime, stale.RunID, 1, 900)

	fresh, err := service.Record(ctx, backupManifest("c", "succeeded", "", time.Now().UTC().Add(-time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	overview, err := operationspostgres.NewJobRepository(runtime).RuntimeOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, alert := range overview.Alerts {
		if alert.AlertID == "ALERT-BACKUP-FAILED" {
			t.Fatalf("fresh backup run %d did not clear alert: %#v", fresh.RunID, alert)
		}
	}

	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE backup_runs SET status='failed' WHERE id=$1`, fresh.RunID); err == nil {
		t.Fatal("backup run facts must be append-only")
	}
	if _, err := runtime.SQL.ExecContext(ctx, `DELETE FROM backup_runs WHERE id=$1`, fresh.RunID); err == nil {
		t.Fatal("backup run facts must not be deleted")
	}
}

func assertBackupAlert(t *testing.T, runtime *database.Runtime, runID, thresholdCount, thresholdSeconds int64) {
	t.Helper()
	overview, err := operationspostgres.NewJobRepository(runtime).RuntimeOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, alert := range overview.Alerts {
		if alert.AlertID != "ALERT-BACKUP-FAILED" {
			continue
		}
		if alert.ResourceType != "backup_run" || alert.ResourceID != runID || alert.AffectedCount != 1 ||
			alert.PolicyVersion != "p0-operational-alerts-v1" || alert.Owner != "hotkey-oncall" ||
			alert.ThresholdCount != thresholdCount || alert.ThresholdSeconds != thresholdSeconds || alert.TriggeredAt.IsZero() {
			t.Fatalf("backup alert=%#v", alert)
		}
		encoded, err := json.Marshal(alert)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "postgres_backup_failed") || strings.Contains(string(encoded), "must-not-leak") {
			t.Fatalf("backup alert leaked private manifest facts: %s", encoded)
		}
		return
	}
	t.Fatalf("backup alert not found: %#v", overview.Alerts)
}

func backupManifest(marker, status, failureCode string, recoveryPoint time.Time) []byte {
	started := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	completed := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	recovery := "null"
	if !recoveryPoint.IsZero() {
		recovery = `"` + recoveryPoint.Format(time.RFC3339Nano) + `"`
	}
	failure := "null"
	if failureCode != "" {
		failure = `"` + failureCode + `"`
	}
	return []byte(`{"version":"hotkey-backup-run-v1","run_sha256":"` + strings.Repeat(marker, 64) + `","git_revision":"` + strings.Repeat("d", 40) + `","status":"` + status + `","recovery_point_at":` + recovery + `,"started_at":"` + started + `","completed_at":"` + completed + `","failure_code":` + failure + `,"assets":[{"name":"postgres_facts","count":1,"sha256":"` + strings.Repeat("1", 64) + `"},{"name":"minio_evidence","count":1,"sha256":"` + strings.Repeat("2", 64) + `"},{"name":"vault_all_files","count":1,"sha256":"` + strings.Repeat("3", 64) + `"},{"name":"vault_manual_regions","count":1,"sha256":"` + strings.Repeat("4", 64) + `"},{"name":"river_jobs_attempts","count":1,"sha256":"` + strings.Repeat("5", 64) + `"}]}`)
}
