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
	return runtime
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
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	suffix := fmt.Sprintf("deny-%d", time.Now().UnixNano())
	var policyID int64
	if err := databaseHandle.QueryRow(`
INSERT INTO source_rights_policies (
  recorded_by_user_id,approved_by_user_id,idempotency_key,command_fingerprint,source_connection_id,
  scope_type,scope_subject,policy_revision,priority,basis_summary,policy_hash,effective_at
) VALUES ($1,$1,$2,$3,$4,'observation',$5,1,400,'reconciliation deny fixture',$6,$7) RETURNING id`,
		fixture.AdministratorID, "policy-"+suffix, reconciliationSHA256("policy-command-"+suffix), fixture.SourceID,
		"observation-"+suffix, reconciliationSHA256("policy-"+suffix), now.Add(-time.Hour)).Scan(&policyID); err != nil {
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
		now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
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
