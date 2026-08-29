package application

import (
	"context"
	"strings"
	"testing"
	"time"
)

type backupRunStoreFake struct {
	recorded int
	command  BackupRunCommand
}

func (store *backupRunStoreFake) RecordBackupRun(_ context.Context, command BackupRunCommand) (BackupRunReceiptDTO, error) {
	store.recorded++
	store.command = command
	return BackupRunReceiptDTO{RunID: 41, RunSHA256: command.RunSHA256, Status: command.Status}, nil
}

func TestBackupRunServiceAcceptsOnlyCompleteBoundedSuccessManifest(t *testing.T) {
	store := &backupRunStoreFake{}
	service, err := NewBackupRunService(store)
	if err != nil {
		t.Fatal(err)
	}
	payload := backupRunManifestFixture("succeeded", "", time.Now().UTC().Add(-time.Minute))
	receipt, err := service.Record(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if store.recorded != 1 || receipt.RunID != 41 || store.command.AssetCount != 5 ||
		len(store.command.ManifestSHA256) != 64 || store.command.RecoveryPointAt == nil {
		t.Fatalf("recorded command=%+v receipt=%+v", store.command, receipt)
	}

	invalid := strings.Replace(string(payload), `"vault_manual_regions"`, `"must-not-leak"`, 1)
	if _, err := service.Record(context.Background(), []byte(invalid)); err == nil {
		t.Fatal("success manifest accepted an unknown asset")
	}
	if store.recorded != 1 {
		t.Fatal("invalid manifest reached the durable store")
	}
}

func TestBackupRunServiceRequiresBoundedFailureCodeAndStrictJSON(t *testing.T) {
	store := &backupRunStoreFake{}
	service, err := NewBackupRunService(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(context.Background(), backupRunManifestFixture("failed", "postgres_backup_failed", time.Time{})); err != nil {
		t.Fatal(err)
	}
	invalidCode := backupRunManifestFixture("failed", "password=must-not-leak", time.Time{})
	if _, err := service.Record(context.Background(), invalidCode); err == nil {
		t.Fatal("failure manifest accepted an unbounded failure detail")
	}
	withUnknownField := append(backupRunManifestFixture("failed", "backup_timeout", time.Time{})[:len(backupRunManifestFixture("failed", "backup_timeout", time.Time{}))-1], []byte(`,"secret":"must-not-leak"}`)...)
	if _, err := service.Record(context.Background(), withUnknownField); err == nil {
		t.Fatal("backup manifest accepted an unknown field")
	}
	now := time.Now().UTC()
	future := backupRunManifestFixtureWithTimes(
		"failed", "backup_timeout", time.Time{}, now.Add(-time.Minute), now.Add(time.Hour),
	)
	if _, err := service.Record(context.Background(), future); err == nil {
		t.Fatal("backup manifest accepted a future completion time")
	}
	if store.recorded != 1 {
		t.Fatalf("recorded=%d", store.recorded)
	}
}

func backupRunManifestFixture(status, failureCode string, recoveryPoint time.Time) []byte {
	now := time.Now().UTC()
	return backupRunManifestFixtureWithTimes(status, failureCode, recoveryPoint, now.Add(-2*time.Minute), now.Add(-time.Minute))
}

func backupRunManifestFixtureWithTimes(status, failureCode string, recoveryPoint, startedAt, completedAt time.Time) []byte {
	started := startedAt.Format(time.RFC3339Nano)
	completed := completedAt.Format(time.RFC3339Nano)
	recovery := "null"
	if !recoveryPoint.IsZero() {
		recovery = `"` + recoveryPoint.Format(time.RFC3339Nano) + `"`
	}
	failure := "null"
	if failureCode != "" {
		failure = `"` + failureCode + `"`
	}
	return []byte(`{"version":"hotkey-backup-run-v1","run_sha256":"` + strings.Repeat("a", 64) + `","git_revision":"` + strings.Repeat("b", 40) + `","status":"` + status + `","recovery_point_at":` + recovery + `,"started_at":"` + started + `","completed_at":"` + completed + `","failure_code":` + failure + `,"assets":[{"name":"postgres_facts","count":1,"sha256":"` + strings.Repeat("1", 64) + `"},{"name":"minio_evidence","count":1,"sha256":"` + strings.Repeat("2", 64) + `"},{"name":"vault_all_files","count":1,"sha256":"` + strings.Repeat("3", 64) + `"},{"name":"vault_manual_regions","count":1,"sha256":"` + strings.Repeat("4", 64) + `"},{"name":"river_jobs_attempts","count":1,"sha256":"` + strings.Repeat("5", 64) + `"}]}`)
}
