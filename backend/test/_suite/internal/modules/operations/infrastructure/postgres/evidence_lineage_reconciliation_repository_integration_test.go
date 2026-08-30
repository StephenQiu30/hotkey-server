//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"testing"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

type reconciliationRawObjectInspectorFake struct {
	inspections map[string]evidenceLineageAssetInspectionRecord
	objects     []evidenceLineageStoredAssetRecord
}

func (fake reconciliationRawObjectInspectorFake) InspectRawEvidenceObject(_ context.Context, objectKey string, _ int64) (evidenceLineageAssetInspectionRecord, error) {
	if inspection, found := fake.inspections[objectKey]; found {
		return inspection, nil
	}
	return evidenceLineageAssetInspectionRecord{Exists: false}, nil
}

func (fake reconciliationRawObjectInspectorFake) ListRawEvidenceObjects(context.Context, int) ([]evidenceLineageStoredAssetRecord, error) {
	return append([]evidenceLineageStoredAssetRecord(nil), fake.objects...), nil
}

type reconciliationVaultInspectorFake struct{}

func (reconciliationVaultInspectorFake) InspectVaultProjection(context.Context, string, int64) (evidenceLineageAssetInspectionRecord, error) {
	return evidenceLineageAssetInspectionRecord{Exists: false}, nil
}

func (reconciliationVaultInspectorFake) ListVaultProjections(context.Context, int) ([]evidenceLineageStoredAssetRecord, error) {
	return []evidenceLineageStoredAssetRecord{}, nil
}

func TestEvidenceLineageReconciliationDryRunReportsMissingRawObjectWithoutMutation(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "dry-run")
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{}, reconciliationVaultInspectorFake{})
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Reconcile(ctx, operationsapplication.EvidenceLineageReconciliationCommand{
		Scope: "pg-minio", BatchSize: 10, GracePeriodHours: 24, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Inspection.CandidateCount != 1 || len(result.Inspection.FindingCounts) != 1 ||
		result.Inspection.FindingCounts[0].Finding != "missing" || result.Inspection.FindingCounts[0].Count != 1 {
		t.Fatalf("inspection = %+v", result.Inspection)
	}
	assertReconciliationSnapshotLifecycle(t, runtime.SQL, fixture.SnapshotID, "raw_available", true, "")
	var runCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM evidence_lineage_reconciliation_runs`).Scan(&runCount); err != nil || runCount != 0 {
		t.Fatalf("dry-run persisted %d runs: %v", runCount, err)
	}
}

func TestEvidenceLineageReconciliationQuarantinesMissingRawObjectAndIsAuditable(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "apply")
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{}, reconciliationVaultInspectorFake{})
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("pg-minio"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != "completed" || result.Run.ExaminedCount != 1 || result.Run.FindingCount != 1 || result.Run.RepairedCount != 1 {
		t.Fatalf("run = %+v", result.Run)
	}
	assertReconciliationSnapshotLifecycle(t, runtime.SQL, fixture.SnapshotID, "quarantined", false, "")

	var assetType, finding, lifecycleBefore, lifecycleAfter, repairAction, reasonCode, assetKey string
	if err := runtime.SQL.QueryRow(`
SELECT asset_type,finding,lifecycle_before,lifecycle_after,repair_action,reason_code,btrim(asset_key_sha256)
FROM evidence_lineage_reconciliation_items WHERE run_id=$1`, result.Run.RunID).
		Scan(&assetType, &finding, &lifecycleBefore, &lifecycleAfter, &repairAction, &reasonCode, &assetKey); err != nil {
		t.Fatal(err)
	}
	if assetType != "evidence_snapshot" || finding != "missing" || lifecycleBefore != "raw_available" ||
		lifecycleAfter != "quarantined" || repairAction != "quarantine_asset" || reasonCode != "raw_object_missing" || len(assetKey) != 64 {
		t.Fatalf("finding = %q/%q/%q/%q/%q/%q/%q", assetType, finding, lifecycleBefore, lifecycleAfter, repairAction, reasonCode, assetKey)
	}
}

func TestEvidenceLineageReconciliationQuarantinesDigestMismatchAndNeverMarksItHealthy(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "digest-mismatch")
	observedSHA := reconciliationSHA256("tampered-payload")
	objectKey := "source-raw/v1/" + fmt.Sprint(fixture.SourceID) + "/" + fixture.SnapshotKey[:2] + "/" + fixture.SnapshotKey + ".raw"
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{inspections: map[string]evidenceLineageAssetInspectionRecord{
		objectKey: {Exists: true, SHA256: observedSHA, SizeBytes: 128},
	}}, reconciliationVaultInspectorFake{})
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("pg-minio"))
	if err != nil {
		t.Fatal(err)
	}
	assertReconciliationSnapshotLifecycle(t, runtime.SQL, fixture.SnapshotID, "quarantined", false, "")
	var finding, expected, observed, reason string
	if err := runtime.SQL.QueryRow(`
SELECT finding,btrim(expected_sha256),btrim(observed_sha256),reason_code
FROM evidence_lineage_reconciliation_items WHERE run_id=$1`, result.Run.RunID).
		Scan(&finding, &expected, &observed, &reason); err != nil {
		t.Fatal(err)
	}
	if finding != "digest_mismatch" || expected != fixture.PayloadSHA256 || observed != observedSHA || reason != "raw_object_integrity_mismatch" {
		t.Fatalf("digest finding=%q expected=%q observed=%q reason=%q", finding, expected, observed, reason)
	}
}

func TestEvidenceLineageReconciliationHonorsAuditableApprovedRetentionException(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "retention-exception")
	expireReconciliationSnapshot(t, runtime.SQL, fixture.SnapshotID)
	if _, err := runtime.SQL.Exec(`
INSERT INTO evidence_retention_exceptions (
  evidence_snapshot_id,approved_by_user_id,approval_basis,approved_at
) VALUES ($1,$2,'approved legal hold for reconciliation fixture',CURRENT_TIMESTAMP - interval '1 hour')`,
		fixture.SnapshotID, fixture.AdministratorID); err != nil {
		t.Fatal(err)
	}
	objectKey := "source-raw/v1/" + fmt.Sprint(fixture.SourceID) + "/" + fixture.SnapshotKey[:2] + "/" + fixture.SnapshotKey + ".raw"
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{inspections: map[string]evidenceLineageAssetInspectionRecord{
		objectKey: {Exists: true, SHA256: fixture.PayloadSHA256, SizeBytes: 128},
	}}, reconciliationVaultInspectorFake{})
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("all"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.HealthyCount != 1 || result.Run.FindingCount != 0 || result.Run.RepairedCount != 0 {
		t.Fatalf("retention exception run = %+v", result.Run)
	}
	assertReconciliationSnapshotLifecycle(t, runtime.SQL, fixture.SnapshotID, "raw_available", true, "")
	var finding, reason, basis string
	if err := runtime.SQL.QueryRow(`
SELECT item.finding,item.reason_code,exception.approval_basis
FROM evidence_lineage_reconciliation_items item
JOIN evidence_retention_exceptions exception ON exception.evidence_snapshot_id=item.asset_id
WHERE item.run_id=$1`, result.Run.RunID).Scan(&finding, &reason, &basis); err != nil {
		t.Fatal(err)
	}
	if finding != "healthy" || reason != "approved_retention_exception" || basis != "approved legal hold for reconciliation fixture" {
		t.Fatalf("retention exception audit=%q/%q/%q", finding, reason, basis)
	}
}

func TestEvidenceLineageReconciliationBlocksWhenStorageInspectorIsUnavailable(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	insertReconciliationRawSnapshotFixture(t, runtime.SQL, "unavailable")
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(NewEvidenceLineageMaintenanceRepository(runtime))
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("pg-minio"))
	if err == nil || len(result.Inspection.Blockers) != 1 || result.Inspection.Blockers[0] != "minio_inspector_unavailable" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestEvidenceLineageReconciliationRejectsUnrecordedBackupEvidence(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	insertReconciliationRawSnapshotFixture(t, runtime.SQL, "unrecorded-backup")
	if _, err := runtime.SQL.Exec(`DELETE FROM backup_runs`); err == nil {
		t.Fatal("append-only backup fact unexpectedly deleted")
	}
	if _, err := runtime.SQL.Exec(`ALTER TABLE backup_runs DISABLE TRIGGER backup_runs_append_only`); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`DELETE FROM backup_runs`); err != nil {
		t.Fatal(err)
	}
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{}, reconciliationVaultInspectorFake{})
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("pg-minio"))
	if err == nil || result.Run.RunID != 0 {
		t.Fatalf("unrecorded backup result=%+v error=%v", result, err)
	}
	var runCount int
	if queryErr := runtime.SQL.QueryRow(`SELECT count(*) FROM evidence_lineage_reconciliation_runs`).Scan(&runCount); queryErr != nil || runCount != 0 {
		t.Fatalf("unrecorded backup persisted %d reconciliation runs: %v", runCount, queryErr)
	}
}

func TestEvidenceLineageReconciliationReportsAuditedRawDeletionAsHealthy(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "audited-raw-deletion")
	insertReconciliationStoreRawDeny(t, runtime.SQL, fixture)
	deletedAt := time.Now().UTC().Truncate(time.Microsecond)
	rawRepository := sourcepostgres.NewRawEvidenceRetentionRepository(runtime)
	candidates, err := rawRepository.ClaimExpired(ctx, deletedAt, 1)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("claim raw deletion candidates=%+v error=%v", candidates, err)
	}
	candidate := candidates[0]
	if err := rawRepository.CompleteDeletion(ctx, sourceapplication.CompleteRawEvidenceDeletionCommand{
		SnapshotID: candidate.SnapshotID, AttemptNo: candidate.AttemptNo,
		ObjectKey: candidate.ObjectKey, PayloadSHA256: candidate.PayloadSHA256,
		DeletedAt: deletedAt, AlreadyMissing: true,
	}); err != nil {
		t.Fatal(err)
	}
	recordReconciliationBackupDisposition(t, runtime.SQL, deletedAt)

	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{}, reconciliationVaultInspectorFake{})
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("all"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.ExaminedCount != 1 || result.Run.HealthyCount != 1 || result.Run.FindingCount != 0 || result.Run.RepairedCount != 0 {
		t.Fatalf("audited raw deletion run=%+v", result.Run)
	}
	var finding, reason string
	if err := runtime.SQL.QueryRow(`
SELECT finding,reason_code FROM evidence_lineage_reconciliation_items
WHERE run_id=$1 AND asset_type='evidence_snapshot'`, result.Run.RunID).Scan(&finding, &reason); err != nil {
		t.Fatal(err)
	}
	if finding != "healthy" || reason != "approved_raw_deletion_verified" {
		t.Fatalf("audited raw deletion finding=%q reason=%q", finding, reason)
	}
	objectKey := fmt.Sprintf("source-raw/v1/%d/%s/%s.raw", fixture.SourceID, fixture.SnapshotKey[:2], fixture.SnapshotKey)
	residualRepository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{
		inspections: map[string]evidenceLineageAssetInspectionRecord{
			objectKey: {Exists: true, SHA256: fixture.PayloadSHA256, SizeBytes: 128},
		},
	}, reconciliationVaultInspectorFake{})
	residualService, err := operationsapplication.NewEvidenceLineageReconciliationService(residualRepository)
	if err != nil {
		t.Fatal(err)
	}
	residual, err := residualService.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("all"))
	if err != nil {
		t.Fatal(err)
	}
	if residual.Run.HealthyCount != 0 || residual.Run.FindingCount != 1 {
		t.Fatalf("residual raw deletion run=%+v", residual.Run)
	}
	if err := runtime.SQL.QueryRow(`
SELECT finding,reason_code FROM evidence_lineage_reconciliation_items
WHERE run_id=$1 AND asset_type='evidence_snapshot'`, residual.Run.RunID).Scan(&finding, &reason); err != nil {
		t.Fatal(err)
	}
	if finding != "policy_blocked" || reason != "audited_raw_deletion_has_residual_object" {
		t.Fatalf("residual raw deletion finding=%q reason=%q", finding, reason)
	}
}

func TestEvidenceLineageReconciliationRejectsUndisposedBackupCopiesAfterDeletion(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "undisposed-backup")
	insertReconciliationStoreRawDeny(t, runtime.SQL, fixture)
	deletedAt := time.Now().UTC().Truncate(time.Microsecond)
	rawRepository := sourcepostgres.NewRawEvidenceRetentionRepository(runtime)
	candidates, err := rawRepository.ClaimExpired(ctx, deletedAt, 1)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("claim raw deletion candidates=%+v error=%v", candidates, err)
	}
	candidate := candidates[0]
	if err := rawRepository.CompleteDeletion(ctx, sourceapplication.CompleteRawEvidenceDeletionCommand{
		SnapshotID: candidate.SnapshotID, AttemptNo: candidate.AttemptNo,
		ObjectKey: candidate.ObjectKey, PayloadSHA256: candidate.PayloadSHA256,
		DeletedAt: deletedAt, AlreadyMissing: true,
	}); err != nil {
		t.Fatal(err)
	}
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{}, reconciliationVaultInspectorFake{})
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("all"))
	if err == nil || result.Run.RunID != 0 {
		t.Fatalf("undisposed backup reconciliation=%+v error=%v", result, err)
	}
	var runCount int64
	if queryErr := runtime.SQL.QueryRow(`SELECT count(*) FROM evidence_lineage_reconciliation_runs`).Scan(&runCount); queryErr != nil || runCount != 0 {
		t.Fatalf("undisposed backup persisted runs=%d error=%v", runCount, queryErr)
	}
	recordReconciliationBackupDisposition(t, runtime.SQL, time.Now().UTC())
	result, err = service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("all"))
	if err != nil || result.Run.RunID <= 0 || result.Run.BackupDispositionCount != 1 ||
		result.Run.HealthyCount != 1 || result.Run.FindingCount != 0 {
		t.Fatalf("disposed backup reconciliation=%+v error=%v", result, err)
	}
}

func TestEvidenceLineageReconciliationResumeIgnoresBackupRunsRecordedAfterFence(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "post-fence-backup")
	insertReconciliationStoreRawDeny(t, runtime.SQL, fixture)
	deletedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	rawRepository := sourcepostgres.NewRawEvidenceRetentionRepository(runtime)
	candidates, err := rawRepository.ClaimExpired(ctx, deletedAt, 1)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("claim raw deletion candidates=%+v error=%v", candidates, err)
	}
	candidate := candidates[0]
	if err := rawRepository.CompleteDeletion(ctx, sourceapplication.CompleteRawEvidenceDeletionCommand{
		SnapshotID: candidate.SnapshotID, AttemptNo: candidate.AttemptNo,
		ObjectKey: candidate.ObjectKey, PayloadSHA256: candidate.PayloadSHA256,
		DeletedAt: deletedAt, AlreadyMissing: true,
	}); err != nil {
		t.Fatal(err)
	}
	recordReconciliationBackupDisposition(t, runtime.SQL, time.Now().UTC())
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{}, reconciliationVaultInspectorFake{})
	command := validEvidenceLineageReconciliationApplyCommand("all")
	run, err := repository.StartEvidenceLineageReconciliation(ctx, operationsapplication.StartEvidenceLineageReconciliationCommand{
		Scope: command.Scope, BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
		OperatorID: command.OperatorID, ReviewerID: command.ReviewerID,
		BinarySHA256: command.BinarySHA256, SchemaSHA256: command.SchemaSHA256,
		ConfigurationSHA256: command.ConfigurationSHA256, BackupEvidenceSHA256: command.BackupEvidenceSHA256,
		RehearsalEvidenceSHA256: command.RehearsalEvidenceSHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.BackupDispositionCount != 1 {
		t.Fatalf("initial backup disposition count=%d", run.BackupDispositionCount)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO backup_runs (
  run_sha256,manifest_sha256,git_revision,status,recovery_point_at,started_at,completed_at,asset_count
) VALUES ($1,$2,$3,'succeeded',$4,$5,$6,5)`, reconciliationSHA256("post-fence-backup"),
		reconciliationSHA256("post-fence-backup-manifest"), reconciliationSHA256("post-fence-git-revision")[:40],
		deletedAt.Add(-time.Hour), deletedAt.Add(-2*time.Hour), run.FencedAt.Add(-time.Microsecond)); err != nil {
		t.Fatal(err)
	}
	resumed, err := repository.ResumeEvidenceLineageReconciliation(ctx, operationsapplication.ResumeEvidenceLineageReconciliationCommand{
		RunID: run.RunID, Scope: command.Scope, BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
		OperatorID: command.OperatorID, ReviewerID: command.ReviewerID,
		BinarySHA256: command.BinarySHA256, SchemaSHA256: command.SchemaSHA256,
		ConfigurationSHA256: command.ConfigurationSHA256, BackupEvidenceSHA256: command.BackupEvidenceSHA256,
		RehearsalEvidenceSHA256: command.RehearsalEvidenceSHA256,
	})
	if err != nil || resumed.BackupDispositionCount != 1 || !resumed.FencedAt.Equal(run.FencedAt) {
		t.Fatalf("post-fence backup changed resume receipt=%+v error=%v", resumed, err)
	}
}

func TestEvidenceLineageReconciliationUsesRunTimeFenceAcrossRightsChanges(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "run-time-fence")
	objectKey := fmt.Sprintf("source-raw/v1/%d/%s/%s.raw", fixture.SourceID, fixture.SnapshotKey[:2], fixture.SnapshotKey)
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{
		inspections: map[string]evidenceLineageAssetInspectionRecord{
			objectKey: {Exists: true, SHA256: fixture.PayloadSHA256, SizeBytes: 128},
		},
	}, reconciliationVaultInspectorFake{})
	command := validEvidenceLineageReconciliationApplyCommand("all")
	run, err := repository.StartEvidenceLineageReconciliation(ctx, operationsapplication.StartEvidenceLineageReconciliationCommand{
		Scope: command.Scope, BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
		OperatorID: command.OperatorID, ReviewerID: command.ReviewerID,
		BinarySHA256: command.BinarySHA256, SchemaSHA256: command.SchemaSHA256,
		ConfigurationSHA256: command.ConfigurationSHA256, BackupEvidenceSHA256: command.BackupEvidenceSHA256,
		RehearsalEvidenceSHA256: command.RehearsalEvidenceSHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	insertReconciliationStoreRawDenyAt(t, runtime.SQL, fixture, run.FencedAt.Add(time.Microsecond))
	batch, err := repository.ApplyEvidenceLineageReconciliationBatch(ctx, operationsapplication.EvidenceLineageReconciliationBatchCommand{
		RunID: run.RunID, Scope: command.Scope, FencedAt: run.FencedAt,
		AfterAssetCursor: 0, BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.ExaminedCount != 1 || batch.HealthyCount != 1 || batch.FindingCount != 0 {
		t.Fatalf("time-fenced batch=%+v", batch)
	}
	var finding, reason string
	if err := runtime.SQL.QueryRow(`
SELECT finding,reason_code FROM evidence_lineage_reconciliation_items WHERE run_id=$1`, run.RunID).Scan(&finding, &reason); err != nil {
		t.Fatal(err)
	}
	if finding != "healthy" || reason != "asset_verified" {
		t.Fatalf("time-fenced finding=%q reason=%q", finding, reason)
	}
}

func TestEvidenceLineageReconciliationDoesNotTrustDeletionAuditRecordedAfterRunFence(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "post-fence-deletion")
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{}, reconciliationVaultInspectorFake{})
	command := validEvidenceLineageReconciliationApplyCommand("all")
	run, err := repository.StartEvidenceLineageReconciliation(ctx, operationsapplication.StartEvidenceLineageReconciliationCommand{
		Scope: command.Scope, BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
		OperatorID: command.OperatorID, ReviewerID: command.ReviewerID,
		BinarySHA256: command.BinarySHA256, SchemaSHA256: command.SchemaSHA256,
		ConfigurationSHA256: command.ConfigurationSHA256, BackupEvidenceSHA256: command.BackupEvidenceSHA256,
		RehearsalEvidenceSHA256: command.RehearsalEvidenceSHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	deletedAt := run.FencedAt.Add(time.Microsecond)
	insertReconciliationStoreRawDenyAt(t, runtime.SQL, fixture, deletedAt)
	rawRepository := sourcepostgres.NewRawEvidenceRetentionRepository(runtime)
	candidates, err := rawRepository.ClaimExpired(ctx, deletedAt, 1)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("claim post-fence deletion candidates=%+v error=%v", candidates, err)
	}
	candidate := candidates[0]
	if err := rawRepository.CompleteDeletion(ctx, sourceapplication.CompleteRawEvidenceDeletionCommand{
		SnapshotID: candidate.SnapshotID, AttemptNo: candidate.AttemptNo,
		ObjectKey: candidate.ObjectKey, PayloadSHA256: candidate.PayloadSHA256,
		DeletedAt: deletedAt, AlreadyMissing: true,
	}); err != nil {
		t.Fatal(err)
	}
	batch, err := repository.ApplyEvidenceLineageReconciliationBatch(ctx, operationsapplication.EvidenceLineageReconciliationBatchCommand{
		RunID: run.RunID, Scope: command.Scope, FencedAt: run.FencedAt,
		AfterAssetCursor: 0, BatchSize: command.BatchSize, GracePeriodHours: command.GracePeriodHours,
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.ExaminedCount != 1 || batch.HealthyCount != 0 || batch.FindingCount != 1 {
		t.Fatalf("post-fence deletion batch=%+v", batch)
	}
	var finding, reason string
	if err := runtime.SQL.QueryRow(`
SELECT finding,reason_code FROM evidence_lineage_reconciliation_items WHERE run_id=$1`, run.RunID).Scan(&finding, &reason); err != nil {
		t.Fatal(err)
	}
	if finding != "policy_blocked" || reason != "raw_deletion_not_verified_at_fence" {
		t.Fatalf("post-fence deletion finding=%q reason=%q", finding, reason)
	}
}

func TestEvidenceLineageReconciliationClassifiesUntrackedObjectsByGracePeriodWithoutDeletingThem(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	oldObject := "source-raw/v1/orphan/aa/" + reconciliationSHA256("old-orphan") + ".raw"
	newObject := "source-raw/v1/orphan/bb/" + reconciliationSHA256("new-orphan") + ".raw"
	repository := newEvidenceLineageMaintenanceRepository(runtime, reconciliationRawObjectInspectorFake{objects: []evidenceLineageStoredAssetRecord{
		{Locator: oldObject, ModifiedAt: time.Now().UTC().Add(-48 * time.Hour)},
		{Locator: newObject, ModifiedAt: time.Now().UTC().Add(-time.Hour)},
	}}, reconciliationVaultInspectorFake{})
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(repository)
	if err != nil {
		t.Fatal(err)
	}

	dryRun, err := service.Reconcile(ctx, operationsapplication.EvidenceLineageReconciliationCommand{
		Scope: "pg-minio", BatchSize: 1, GracePeriodHours: 24, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dryRun.Inspection.CandidateCount != 2 || findingCount(dryRun.Inspection, "orphan_expired") != 1 ||
		findingCount(dryRun.Inspection, "orphan_within_grace") != 1 {
		t.Fatalf("orphan inspection = %+v", dryRun.Inspection)
	}
	applyResult, err := service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("pg-minio"))
	if err != nil {
		t.Fatal(err)
	}
	if applyResult.Run.ExaminedCount != 2 || applyResult.Run.FindingCount != 2 || applyResult.Run.RepairedCount != 0 {
		t.Fatalf("orphan run = %+v", applyResult.Run)
	}
	var count int
	if err := runtime.SQL.QueryRow(`
SELECT count(*) FROM evidence_lineage_reconciliation_items
WHERE run_id=$1 AND asset_type='raw_object_orphan' AND asset_id IS NULL
  AND source_connection_id IS NULL AND repair_action='none'`, applyResult.Run.RunID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("orphan audit count=%d error=%v", count, err)
	}
}

func TestEvidenceLineageReconciliationBlocksRawAssetAfterCurrentStorageRightIsDenied(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "rights-denied")
	insertReconciliationStoreRawDeny(t, runtime.SQL, fixture)
	service, err := operationsapplication.NewEvidenceLineageReconciliationService(NewEvidenceLineageMaintenanceRepository(runtime))
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Reconcile(ctx, validEvidenceLineageReconciliationApplyCommand("rights-retention"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.FindingCount != 1 || result.Run.RepairedCount != 1 {
		t.Fatalf("rights run = %+v", result.Run)
	}
	assertReconciliationSnapshotLifecycle(t, runtime.SQL, fixture.SnapshotID, "policy_blocked", false, "")
	var finding, reason string
	if err := runtime.SQL.QueryRow(`SELECT finding,reason_code FROM evidence_lineage_reconciliation_items WHERE run_id=$1`, result.Run.RunID).Scan(&finding, &reason); err != nil {
		t.Fatal(err)
	}
	if finding != "policy_blocked" || reason != "current_storage_right_denied" {
		t.Fatalf("rights finding=%q reason=%q", finding, reason)
	}
}

func TestEvidenceLineageSampleTraversesEventDocumentObservationAndRejectsCurrentAuthorizationMismatch(t *testing.T) {
	ctx := context.Background()
	runtime := openEvidenceLineageReconciliationRuntime(t, ctx)
	defer runtime.Close()
	fixture := insertReconciliationRawSnapshotFixture(t, runtime.SQL, "event-lineage")
	eventID, documentVersionID, observationID := insertReconciliationEventLineage(t, runtime.SQL, fixture)

	var sampledEventID, sampledVersionID, sampledObservationID, sampledSnapshotID int64
	var payloadSHA string
	var hashMatches, lifecycleValid, authorizationValid bool
	readSample := func() {
		t.Helper()
		if err := runtime.SQL.QueryRow(`
SELECT event.id,version.id,observation.id,snapshot.id,btrim(snapshot.payload_sha256),
       reference.selected_payload_sha256=snapshot.payload_sha256,
       snapshot.lifecycle_state='raw_available',
       current_rights_action_is_allowed(
         snapshot.source_connection_id,
         'raw_response',btrim(snapshot.snapshot_key),snapshot.payload_sha256,'store_raw',CURRENT_TIMESTAMP
       ) AND current_rights_retention_days(
         snapshot.source_connection_id,'raw_response',btrim(snapshot.snapshot_key),snapshot.payload_sha256,CURRENT_TIMESTAMP
       ) IS NOT NULL
FROM micro_events event
JOIN micro_event_members event_member ON event_member.micro_event_id=event.id AND event_member.active
JOIN content_family_members family_member ON family_member.family_id=event_member.content_family_id AND family_member.active
JOIN document_versions version ON version.id=family_member.document_version_id
JOIN source_observations observation ON observation.id=version.source_observation_id
JOIN source_observation_evidences reference
  ON reference.source_observation_id=observation.id
 AND reference.source_connection_id=observation.source_connection_id
JOIN evidence_snapshots snapshot
  ON snapshot.id=reference.evidence_snapshot_id
 AND snapshot.source_connection_id=reference.source_connection_id
WHERE event.id=$1 AND version.id=$2`, eventID, documentVersionID).
			Scan(&sampledEventID, &sampledVersionID, &sampledObservationID, &sampledSnapshotID,
				&payloadSHA, &hashMatches, &lifecycleValid, &authorizationValid); err != nil {
			t.Fatal(err)
		}
	}
	readSample()
	if sampledEventID != eventID || sampledVersionID != documentVersionID || sampledObservationID != observationID ||
		sampledSnapshotID != fixture.SnapshotID || payloadSHA != fixture.PayloadSHA256 || !hashMatches || !lifecycleValid || !authorizationValid {
		t.Fatalf("lineage sample event=%d version=%d observation=%d snapshot=%d hash=%q valid=%v/%v/%v",
			sampledEventID, sampledVersionID, sampledObservationID, sampledSnapshotID, payloadSHA,
			hashMatches, lifecycleValid, authorizationValid)
	}

	insertReconciliationStoreRawDeny(t, runtime.SQL, fixture)
	readSample()
	if authorizationValid {
		t.Fatal("event lineage sample remained valid after current raw-storage authorization was denied")
	}
}

type reconciliationRawSnapshotFixture struct {
	SnapshotID      int64
	AdministratorID int64
	SourceID        int64
	SnapshotKey     string
	PayloadSHA256   string
}

func openEvidenceLineageReconciliationRuntime(t *testing.T, ctx context.Context) *database.Runtime {
	t.Helper()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		runtime.Close()
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if _, err := runtime.SQL.Exec(`
INSERT INTO backup_runs (
  run_sha256,manifest_sha256,git_revision,status,recovery_point_at,started_at,completed_at,asset_count
) VALUES ($1,$2,$3,'succeeded',$4,$5,$6,5)`, reconciliationSHA256("backup"),
		reconciliationSHA256("backup-manifest"), reconciliationSHA256("git-revision")[:40],
		now.Add(-time.Minute), now.Add(-2*time.Minute), now); err != nil {
		runtime.Close()
		t.Fatal(err)
	}
	return runtime
}

func recordReconciliationBackupDisposition(t *testing.T, databaseHandle *sql.DB, disposedAt time.Time) {
	t.Helper()
	backupSHA256 := reconciliationSHA256("backup")
	if _, err := databaseHandle.Exec(`
INSERT INTO backup_retention_dispositions (
  disposition_sha256,manifest_sha256,backup_run_id,backup_run_sha256,deletion_evidence_sha256,
  status,reason_code,operator_record_id,reviewer_record_id,disposed_at
)
SELECT $1,$2,id,run_sha256,$3,'disposed','rights_revoked','backup-operator','backup-reviewer',$4
FROM backup_runs WHERE run_sha256=$5`, reconciliationSHA256("backup-disposition"),
		reconciliationSHA256("backup-disposition-manifest"), reconciliationSHA256("deletion-evidence"),
		disposedAt.UTC(), backupSHA256); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := databaseHandle.QueryRow(`
SELECT count(*) FROM backup_retention_dispositions
WHERE backup_run_sha256=$1 AND status='disposed'`, backupSHA256).Scan(&count); err != nil || count != 1 {
		t.Fatalf("backup disposition count=%d error=%v", count, err)
	}
}

func insertReconciliationRawSnapshotFixture(t *testing.T, databaseHandle *sql.DB, label string) reconciliationRawSnapshotFixture {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	suffix := fmt.Sprintf("%s-%d", label, time.Now().UnixNano())
	var administratorID, sourceID int64
	if err := databaseHandle.QueryRow(`
INSERT INTO users (email,password_hash,display_name,role,status)
VALUES ($1,'fixture-password-hash','Maintenance Administrator','admin','active') RETURNING id`, suffix+"@example.test").Scan(&administratorID); err != nil {
		t.Fatal(err)
	}
	if err := databaseHandle.QueryRow(`
INSERT INTO source_connections (source_type,name,endpoint)
VALUES ('rss',$1,$2) RETURNING id`, "reconciliation-"+suffix, "https://feed.example.test/"+suffix).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	payloadSHA := reconciliationSHA256("payload-" + suffix)
	snapshotKey := reconciliationSHA256("snapshot-" + suffix)
	policyHash := reconciliationSHA256("policy-" + suffix)
	var policyID, decisionBatchID, storeDecisionID, retainDecisionID int64
	if err := databaseHandle.QueryRow(`
INSERT INTO source_rights_policies (
  recorded_by_user_id,approved_by_user_id,idempotency_key,command_fingerprint,source_connection_id,
  scope_type,scope_subject,policy_revision,priority,basis_summary,policy_hash,effective_at
) VALUES ($1,$1,$2,$3,$4,'source_endpoint',$5,1,300,'reconciliation fixture',$6,$7) RETURNING id`,
		administratorID, "policy-"+suffix, reconciliationSHA256("policy-command-"+suffix), sourceID,
		"source-endpoint-"+suffix, policyHash, now.Add(-time.Hour)).Scan(&policyID); err != nil {
		t.Fatal(err)
	}
	retentionDays := 30
	if err := databaseHandle.QueryRow(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches (
    source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
    recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
  ) VALUES ($1,$2,1,'raw_response',$3,$4,$5,$6,$7,2) RETURNING id
), store_decision AS (
  INSERT INTO source_rights_decisions (
    decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
    priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,evaluator,evaluated_at,effective_from
  ) SELECT id,$1,$2,1,'source_endpoint',$8,300,'reconciliation fixture','raw_response',$3,$4,
           'store_raw','allow','fixture',$9,$9 FROM decision_batch RETURNING id
), retain_decision AS (
  INSERT INTO source_rights_decisions (
    decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
    priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,evaluator,evaluated_at,effective_from,retention_days
  ) SELECT id,$1,$2,1,'source_endpoint',$8,300,'reconciliation fixture','raw_response',$3,$4,
           'retain','allow','fixture',$9,$9,$10 FROM decision_batch RETURNING id
)
SELECT decision_batch.id,store_decision.id,retain_decision.id
FROM decision_batch CROSS JOIN store_decision CROSS JOIN retain_decision`,
		sourceID, policyID, snapshotKey, payloadSHA, administratorID, "batch-"+suffix,
		reconciliationSHA256("batch-command-"+suffix), "source-endpoint-"+suffix,
		now.Add(-time.Hour), retentionDays).Scan(&decisionBatchID, &storeDecisionID, &retainDecisionID); err != nil {
		t.Fatal(err)
	}
	objectKey := fmt.Sprintf("source-raw/v1/%d/%s/%s.raw", sourceID, snapshotKey[:2], snapshotKey)
	var snapshotID int64
	if err := databaseHandle.QueryRow(`
INSERT INTO evidence_snapshots (
  source_connection_id,store_raw_rights_decision_id,retain_rights_decision_id,snapshot_key,object_key,
  payload_sha256,collector_profile_version,mime_type,size_bytes,response_status,requested_url,final_url,
  captured_at,retention_until,lifecycle_state,available_at
) VALUES ($1,$2,$3,$4,$5,$6,'maintenance-fixture-v1','application/xml',128,200,$7,$7,$8,$9,'raw_available',$8)
RETURNING id`, sourceID, storeDecisionID, retainDecisionID, snapshotKey, objectKey, payloadSHA,
		"https://feed.example.test/"+suffix, now, now.Add(30*24*time.Hour)).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	return reconciliationRawSnapshotFixture{
		SnapshotID: snapshotID, AdministratorID: administratorID, SourceID: sourceID,
		SnapshotKey: snapshotKey, PayloadSHA256: payloadSHA,
	}
}

func insertReconciliationStoreRawDeny(t *testing.T, databaseHandle *sql.DB, fixture reconciliationRawSnapshotFixture) {
	t.Helper()
	insertReconciliationStoreRawDenyAt(t, databaseHandle, fixture, time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond))
}

func insertReconciliationStoreRawDenyAt(t *testing.T, databaseHandle *sql.DB, fixture reconciliationRawSnapshotFixture, effectiveAt time.Time) {
	t.Helper()
	effectiveAt = effectiveAt.UTC().Truncate(time.Microsecond)
	suffix := fmt.Sprintf("deny-%d", time.Now().UnixNano())
	var policyID int64
	if err := databaseHandle.QueryRow(`
INSERT INTO source_rights_policies (
  recorded_by_user_id,approved_by_user_id,idempotency_key,command_fingerprint,source_connection_id,
  scope_type,scope_subject,policy_revision,priority,basis_summary,policy_hash,effective_at
) VALUES ($1,$1,$2,$3,$4,'observation',$5,1,400,'reconciliation deny fixture',$6,$7) RETURNING id`,
		fixture.AdministratorID, "policy-"+suffix, reconciliationSHA256("policy-command-"+suffix), fixture.SourceID,
		"observation-"+suffix, reconciliationSHA256("policy-"+suffix), effectiveAt).Scan(&policyID); err != nil {
		t.Fatal(err)
	}
	if _, err := databaseHandle.Exec(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches (
    source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
    recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
  ) VALUES ($1,$2,1,'raw_response',$3,$4,$5,$6,$7,1) RETURNING id
)
INSERT INTO source_rights_decisions (
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,evaluator,evaluated_at,effective_from
) SELECT id,$1,$2,1,'observation',$8,400,'reconciliation deny fixture','raw_response',$3,$4,
         'store_raw','deny','fixture',$9,$9 FROM decision_batch`,
		fixture.SourceID, policyID, fixture.SnapshotKey, fixture.PayloadSHA256, fixture.AdministratorID,
		"batch-"+suffix, reconciliationSHA256("batch-command-"+suffix), "observation-"+suffix,
		effectiveAt); err != nil {
		t.Fatal(err)
	}
}

func expireReconciliationSnapshot(t *testing.T, databaseHandle *sql.DB, snapshotID int64) {
	t.Helper()
	transaction, err := databaseHandle.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`
UPDATE evidence_snapshots
SET captured_at=CURRENT_TIMESTAMP - interval '3 days',
    created_at=CURRENT_TIMESTAMP - interval '3 days',
    retention_until=CURRENT_TIMESTAMP - interval '1 day',
    available_at=CURRENT_TIMESTAMP - interval '3 days',
    updated_at=CURRENT_TIMESTAMP - interval '1 day'
WHERE id=$1`, snapshotID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func insertReconciliationEventLineage(t *testing.T, databaseHandle *sql.DB, fixture reconciliationRawSnapshotFixture) (int64, int64, int64) {
	t.Helper()
	transaction, err := databaseHandle.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := fmt.Sprintf("lineage-%d", time.Now().UnixNano())
	var observationID, evidenceReferenceID, documentID, documentVersionID int64
	if err := transaction.QueryRow(`
INSERT INTO source_observations (
  source_connection_id,external_id,upstream_identity,source_code,content_type,title,language,
  source_record_url,body_origin,completeness,discovered_at,captured_at
) VALUES ($1,$2,$3,'rss','article','Lineage sample','en',$4,'feed_content','full',$5,$5)
RETURNING id`, fixture.SourceID, suffix, reconciliationSHA256("upstream-"+suffix),
		"https://feed.example.test/items/"+suffix, now).Scan(&observationID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`
INSERT INTO source_observation_evidences (
  source_connection_id,source_observation_id,evidence_snapshot_id,usage,locator_type,
  locator_value,selected_payload_sha256,selector_version
) VALUES ($1,$2,$3,'document_source','whole_payload','$',$4,'lineage-sample-v1')
RETURNING id`, fixture.SourceID, observationID, fixture.SnapshotID, fixture.PayloadSHA256).Scan(&evidenceReferenceID); err != nil {
		t.Fatal(err)
	}
	if evidenceReferenceID <= 0 {
		t.Fatal("evidence reference was not created")
	}
	if err := transaction.QueryRow(`
INSERT INTO documents (source_connection_id,document_key,external_work_id)
VALUES ($1,$2,$3) RETURNING id`, fixture.SourceID, reconciliationSHA256("document-"+suffix), suffix).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`
INSERT INTO document_versions (
  document_id,source_observation_id,revision_no,version_key,body_origin,completeness,word_count,
  language,content_sha256,extractor_version,extractor_profile_version,extractor_profile_sha256,
  lifecycle_state,captured_at
) VALUES ($1,$2,1,$3,'feed_content','full',2,'en',$4,'lineage-v1','lineage-profile-v1',$5,'readable',$6)
RETURNING id`, documentID, observationID, reconciliationSHA256("version-"+suffix),
		reconciliationSHA256("content-"+suffix), reconciliationSHA256("extractor-"+suffix), now).Scan(&documentVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`UPDATE documents SET current_document_version_id=$2 WHERE id=$1`, documentID, documentVersionID); err != nil {
		t.Fatal(err)
	}
	var fingerprintID, familyID, lineageDecisionID int64
	if err := transaction.QueryRow(`
INSERT INTO content_fingerprints (
  source_connection_id,document_version_id,derived_artifact_id,store_derived_rights_decision_id,
  retain_rights_decision_id,profile_version,normalized_content_sha256,simhash_hex,minhash,retention_until
) VALUES ($1,$2,900001,900002,900003,'lineage-sample-v1',$3,'1111111111111111',$4,$5)
RETURNING id`, fixture.SourceID, documentVersionID, reconciliationSHA256("normalized-"+suffix),
		make([]byte, 512), now.Add(24*time.Hour)).Scan(&fingerprintID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`
INSERT INTO content_families (root_document_version_id,lineage_profile_version)
VALUES ($1,'lineage-sample-v1') RETURNING id`, documentVersionID).Scan(&familyID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`
INSERT INTO content_lineage_decisions (
  document_version_id,fingerprint_id,family_id,result_family_version,action,relation,
  hamming_distance,minhash_similarity,decision_profile_version,reason_codes,idempotency_key,command_fingerprint
) VALUES ($1,$2,$3,1,'create','unrelated',64,0,'lineage-sample-v1','["sample"]',$4,$5)
RETURNING id`, documentVersionID, fingerprintID, familyID, "lineage-decision-"+suffix,
		reconciliationSHA256("lineage-decision-"+suffix)).Scan(&lineageDecisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`
INSERT INTO content_family_members (
  family_id,document_version_id,fingerprint_id,lineage_decision_id,lineage_profile_version,relation
) VALUES ($1,$2,$3,$4,'lineage-sample-v1','unrelated')`, familyID, documentVersionID, fingerprintID, lineageDecisionID); err != nil {
		t.Fatal(err)
	}
	var eventID, membershipDecisionID int64
	if err := transaction.QueryRow(`
INSERT INTO micro_events (
  event_key,status,primary_subject_key,primary_action_key,event_started_at,clustering_profile_version
) VALUES ($1,'active','subject:lineage','action:sample',$2,'lineage-sample-v1') RETURNING id`,
		reconciliationSHA256("event-"+suffix), now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.QueryRow(`
INSERT INTO micro_event_membership_decisions (
  content_family_id,document_match_decision_id,monitor_id,monitor_version_id,resulting_micro_event_id,
  result_event_version,action,same_event_score,leading_margin,sparse_similarity,dense_similarity,
  entity_overlap,action_overlap,location_consistency,identifier_consistency,time_similarity,lineage_relation,
  hard_conflict_reasons,clustering_profile_version,reason_codes,idempotency_key,command_fingerprint
) VALUES ($1,900004,900005,900006,$2,1,'create',1,1,1,1,1,1,1,1,1,1,'[]',
          'lineage-sample-v1','["sample"]',$3,$4)
RETURNING id`, familyID, eventID, "event-membership-"+suffix,
		reconciliationSHA256("event-membership-"+suffix)).Scan(&membershipDecisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`
INSERT INTO micro_event_members (
  micro_event_id,content_family_id,membership_decision_id,clustering_profile_version
) VALUES ($1,$2,$3,'lineage-sample-v1')`, eventID, familyID, membershipDecisionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	return eventID, documentVersionID, observationID
}

func assertReconciliationSnapshotLifecycle(t *testing.T, databaseHandle *sql.DB, snapshotID int64, wantLifecycle string, wantAvailable bool, wantFailure string) {
	t.Helper()
	var lifecycle string
	var availableAt sql.NullTime
	var failureCode sql.NullString
	if err := databaseHandle.QueryRow(`SELECT lifecycle_state,available_at,failure_code FROM evidence_snapshots WHERE id=$1`, snapshotID).
		Scan(&lifecycle, &availableAt, &failureCode); err != nil {
		t.Fatal(err)
	}
	if lifecycle != wantLifecycle || availableAt.Valid != wantAvailable || failureCode.String != wantFailure {
		t.Fatalf("snapshot lifecycle=%q available=%v failure=%q, want %q/%v/%q", lifecycle, availableAt.Valid, failureCode.String, wantLifecycle, wantAvailable, wantFailure)
	}
}

func validEvidenceLineageReconciliationApplyCommand(scope string) operationsapplication.EvidenceLineageReconciliationCommand {
	return operationsapplication.EvidenceLineageReconciliationCommand{
		Scope: scope, BatchSize: 10, GracePeriodHours: 24, Apply: true, ConfirmNonEmpty: true,
		OperatorID: "operator-a", ReviewerID: "reviewer-b",
		BinarySHA256: reconciliationSHA256("binary"), SchemaSHA256: reconciliationSHA256("schema"),
		ConfigurationSHA256: reconciliationSHA256("configuration"), BackupEvidenceSHA256: reconciliationSHA256("backup"),
		RehearsalEvidenceSHA256: reconciliationSHA256("rehearsal"),
	}
}

func findingCount(inspection operationsapplication.EvidenceLineageReconciliationInspectionDTO, finding string) int64 {
	for _, value := range inspection.FindingCounts {
		if value.Finding == finding {
			return value.Count
		}
	}
	return 0
}

func reconciliationSHA256(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}
