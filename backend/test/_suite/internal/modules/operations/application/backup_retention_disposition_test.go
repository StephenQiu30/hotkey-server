package application

import (
	"context"
	"strings"
	"testing"
	"time"
)

type backupRetentionDispositionStoreFake struct {
	recorded int
	command  BackupRetentionDispositionCommand
}

func (store *backupRetentionDispositionStoreFake) RecordBackupRetentionDisposition(_ context.Context, command BackupRetentionDispositionCommand) (BackupRetentionDispositionReceiptDTO, error) {
	store.recorded++
	store.command = command
	return BackupRetentionDispositionReceiptDTO{
		DispositionID: 51, BackupRunSHA256: command.BackupRunSHA256, Status: "disposed",
	}, nil
}

func TestBackupRetentionDispositionServiceAcceptsStrictIndependentDispositionManifest(t *testing.T) {
	store := &backupRetentionDispositionStoreFake{}
	service, err := NewBackupRetentionDispositionService(store)
	if err != nil {
		t.Fatal(err)
	}
	payload := backupRetentionDispositionManifestFixture("a", "b", "rights_revoked", time.Now().UTC().Add(-time.Minute))
	receipt, err := service.Record(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if store.recorded != 1 || receipt.DispositionID != 51 || receipt.Status != "disposed" ||
		store.command.OperatorID != "backup-operator" || store.command.ReviewerID != "backup-reviewer" ||
		len(store.command.ManifestSHA256) != 64 || store.command.DisposedAt.IsZero() {
		t.Fatalf("recorded command=%+v receipt=%+v", store.command, receipt)
	}
}

func TestBackupRetentionDispositionServiceRejectsUnboundedOrSelfApprovedManifest(t *testing.T) {
	store := &backupRetentionDispositionStoreFake{}
	service, err := NewBackupRetentionDispositionService(store)
	if err != nil {
		t.Fatal(err)
	}
	valid := string(backupRetentionDispositionManifestFixture("a", "b", "retention_expired", time.Now().UTC().Add(-time.Minute)))
	cases := []struct {
		name    string
		payload string
	}{
		{name: "unknown field", payload: strings.TrimSuffix(valid, "}") + `,"backup_path":"must-not-leak"}`},
		{name: "self approved", payload: strings.Replace(valid, `"backup-reviewer"`, `"backup-operator"`, 1)},
		{name: "unbounded reason", payload: strings.Replace(valid, `"retention_expired"`, `"password=must-not-leak"`, 1)},
		{name: "future disposition", payload: string(backupRetentionDispositionManifestFixture("a", "b", "rights_revoked", time.Now().UTC().Add(time.Hour)))},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Record(context.Background(), []byte(test.payload)); err == nil {
				t.Fatal("invalid backup disposition manifest was accepted")
			}
		})
	}
	if store.recorded != 0 {
		t.Fatalf("invalid manifests reached durable store %d times", store.recorded)
	}
}

func backupRetentionDispositionManifestFixture(dispositionMarker, backupMarker, reason string, disposedAt time.Time) []byte {
	return []byte(`{"version":"hotkey-backup-retention-disposition-v1","disposition_sha256":"` + strings.Repeat(dispositionMarker, 64) +
		`","backup_run_sha256":"` + strings.Repeat(backupMarker, 64) + `","deletion_evidence_sha256":"` + strings.Repeat("c", 64) +
		`","reason_code":"` + reason + `","operator_record_id":"backup-operator","reviewer_record_id":"backup-reviewer","disposed_at":"` +
		disposedAt.Format(time.RFC3339Nano) + `"}`)
}
