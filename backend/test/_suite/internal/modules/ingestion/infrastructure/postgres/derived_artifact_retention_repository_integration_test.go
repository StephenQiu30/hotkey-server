package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	knowledgevault "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/vault"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
)

func TestDerivedArtifactRightsRevocationDeletesOnlyAutomaticVaultProjectionAndKeepsLineage(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	fixture := createDerivedArtifactDocument(t, runtime, "retention-rights-revoked", 301)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	displayDecisionID := createDocumentDisplayDecision(
		t, runtime, fixture.sourceID, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, 2, nil, fixture.persisted.DocumentVersion.ID,
	)
	vaultRoot := t.TempDir()
	writer := knowledgevault.NewWriter(vaultRoot)
	humanBytes := "# 人工记录\n\n该文件不属于自动派生物，必须原样保留。\n"
	humanPath, err := writer.Write("events", "retention-human", humanBytes)
	if err != nil {
		t.Fatal(err)
	}
	profile := strings.Repeat("9", 64)
	projected, err := newDerivedArtifactSaga(
		t, runtime, newKnowledgeProjectionPublisher(t, vaultRoot), fixture.documentVersions,
	).Project(ctx, derivedArtifactProjectCommand(
		fixture, profile, []byte("# Automatic\n\nrevocable projection\n"),
		storeDecisionID, retainDecisionID, &displayDecisionID,
	))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	denyPolicy := createDocumentObservationRightsPolicy(
		t, runtime, fixture.sourceID, fixture.observationID, 3, at.Add(-time.Hour),
	)
	insertDocumentRightsDecisionWithOutcome(
		t, runtime, denyPolicy, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, "store_derived", "deny", nil, nil,
		fixture.persisted.DocumentVersion.ID,
	)
	if _, err := runtime.SQL.Exec(`
UPDATE derived_artifacts
SET lifecycle_state='retention_blocked',active=false,available_at=NULL,failure_code=NULL,updated_at=$2
WHERE id=$1`, projected.Artifact.ID, at); err != nil {
		t.Fatalf("apply reconciliation-first artifact block: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_versions SET version=version+1,lifecycle_state='retention_blocked'
WHERE id=$1 AND lifecycle_state='readable'`, fixture.persisted.DocumentVersion.ID); err != nil {
		t.Fatalf("apply reconciliation-first document block: %v", err)
	}
	service, err := ingestionapplication.NewDerivedArtifactRetentionService(ingestionapplication.DerivedArtifactRetentionDependencies{
		Repository: ingestionpostgres.NewDerivedArtifactRetentionRepository(runtime), Deleter: writer,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Run(ctx, ingestionapplication.RunDerivedArtifactRetentionCommand{At: at, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Claimed != 1 || result.Deleted != 1 || result.Failed != 0 || !projected.Artifact.RetentionUntil.After(at) {
		t.Fatalf("retention result = %#v, retention_until=%s at=%s", result, projected.Artifact.RetentionUntil, at)
	}
	projectionPath := filepath.Join(vaultRoot, filepath.FromSlash(derivedArtifactFixturePath(
		fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, profile,
	)))
	if _, err := os.Stat(projectionPath); !os.IsNotExist(err) {
		t.Fatalf("revoked automatic projection still exists: %v", err)
	}
	retainedHuman, err := os.ReadFile(humanPath)
	if err != nil || string(retainedHuman) != humanBytes {
		t.Fatalf("human Vault file changed = %q/%v", retainedHuman, err)
	}
	var artifactState, documentState, vaultRelativePath, artifactSHA string
	var claimedReason, succeededReason string
	var sizeBytes int64
	if err := runtime.SQL.QueryRow(`
SELECT lifecycle_state,vault_relative_path,btrim(sha256),size_bytes
FROM derived_artifacts WHERE id=$1`, projected.Artifact.ID).Scan(
		&artifactState, &vaultRelativePath, &artifactSHA, &sizeBytes,
	); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state FROM document_versions WHERE id=$1`, fixture.persisted.DocumentVersion.ID).Scan(&documentState); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
SELECT max(reason_code) FILTER (WHERE event_type='delete_claimed'),
       max(reason_code) FILTER (WHERE event_type='delete_succeeded')
FROM derived_artifact_deletion_audits WHERE derived_artifact_id=$1`, projected.Artifact.ID).Scan(
		&claimedReason, &succeededReason,
	); err != nil {
		t.Fatal(err)
	}
	if artifactState != ingestionapplication.DerivedArtifactTombstoned || documentState != ingestionapplication.DocumentRetentionBlocked ||
		vaultRelativePath != derivedArtifactFixturePath(fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, profile) ||
		artifactSHA != projected.Artifact.SHA256 || sizeBytes != projected.Artifact.SizeBytes ||
		claimedReason != ingestionapplication.DerivedArtifactDeleteRightsRevoked || succeededReason != claimedReason {
		t.Fatalf("retained lineage = artifact:%s document:%s path:%s sha:%s size:%d reasons:%s/%s",
			artifactState, documentState, vaultRelativePath, artifactSHA, sizeBytes, claimedReason, succeededReason)
	}
	backupEvidenceSHA256 := strings.Repeat("b", 64)
	if _, err := runtime.SQL.Exec(`
INSERT INTO backup_runs (
  run_sha256,manifest_sha256,git_revision,status,recovery_point_at,started_at,completed_at,asset_count
) VALUES ($1,$2,$3,'succeeded',$4,$5,$6,5)`, backupEvidenceSHA256, strings.Repeat("c", 64),
		strings.Repeat("d", 40), at.Add(-3*time.Minute), at.Add(-5*time.Minute), at.Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	reconciliationRepository, err := operationspostgres.NewEvidenceLineageMaintenanceRepositoryWithVault(runtime, vaultRoot)
	if err != nil {
		t.Fatal(err)
	}
	reconciliation, err := operationsapplication.NewEvidenceLineageReconciliationService(reconciliationRepository)
	if err != nil {
		t.Fatal(err)
	}
	reconciliationResult, err := reconciliation.Reconcile(ctx, operationsapplication.EvidenceLineageReconciliationCommand{
		Scope: "pg-vault", BatchSize: 10, GracePeriodHours: 24, Apply: true, ConfirmNonEmpty: true,
		OperatorID: "retention-operator", ReviewerID: "retention-reviewer",
		BinarySHA256: strings.Repeat("1", 64), SchemaSHA256: strings.Repeat("2", 64),
		ConfigurationSHA256: strings.Repeat("3", 64), BackupEvidenceSHA256: backupEvidenceSHA256,
		RehearsalEvidenceSHA256: strings.Repeat("4", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reconciliationResult.Run.ExaminedCount != 1 || reconciliationResult.Run.HealthyCount != 1 ||
		reconciliationResult.Run.FindingCount != 0 || reconciliationResult.Run.RepairedCount != 0 {
		t.Fatalf("post-deletion Vault reconciliation = %#v", reconciliationResult.Run)
	}
	var reconciliationFinding, reconciliationReason string
	if err := runtime.SQL.QueryRow(`
SELECT finding,reason_code FROM evidence_lineage_reconciliation_items
WHERE run_id=$1 AND asset_type='derived_artifact'`, reconciliationResult.Run.RunID).Scan(
		&reconciliationFinding, &reconciliationReason,
	); err != nil {
		t.Fatal(err)
	}
	if reconciliationFinding != "healthy" || reconciliationReason != "approved_vault_deletion_verified" {
		t.Fatalf("post-deletion Vault finding=%q reason=%q", reconciliationFinding, reconciliationReason)
	}
	replayed, err := service.Run(ctx, ingestionapplication.RunDerivedArtifactRetentionCommand{At: at.Add(time.Minute), Limit: 10})
	if err != nil || replayed.Claimed != 0 || replayed.Deleted != 0 || replayed.Failed != 0 {
		t.Fatalf("retention replay = %#v/%v", replayed, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE derived_artifact_deletion_audits SET reason_code='MUTATED' WHERE derived_artifact_id=$1`, projected.Artifact.ID); err == nil {
		t.Fatal("derived artifact deletion audit accepted mutation")
	}
}
