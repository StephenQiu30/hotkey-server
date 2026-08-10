//go:build integration

package postgres

import (
	"context"
	"testing"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestEvidenceLineageBackfillRepositoryDryRunAndConfirmedApplyAreConservativeAndResumable(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	var sourceOne, sourceTwo int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO source_connections (source_type,name,endpoint) VALUES ('rss','migration-a','https://a.example/feed') RETURNING id`).Scan(&sourceOne); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO source_connections (source_type,name,endpoint) VALUES ('rss','migration-b','https://b.example/feed') RETURNING id`).Scan(&sourceTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO contents (source_connection_id,external_id,content_type,title,canonical_url,published_at,fetched_at,dedupe_key) VALUES ($1,'legacy-1','article','legacy','https://a.example/legacy',now(),now(),$2)`, sourceOne, repeatMaintenanceSHA("1")); err != nil {
		t.Fatal(err)
	}

	repository := NewEvidenceLineageMaintenanceRepository(runtime)
	service, err := operationsapplication.NewEvidenceLineageMaintenanceService(repository)
	if err != nil {
		t.Fatal(err)
	}
	dryRun, err := service.Backfill(ctx, operationsapplication.EvidenceLineageBackfillCommand{Phase: "source", BatchSize: 1, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dryRun.Inspection.CandidateCount != 2 || dryRun.Inspection.CatalogFingerprint == "" {
		t.Fatalf("dry-run inspection = %+v", dryRun.Inspection)
	}
	var runCount int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM evidence_lineage_migration_runs`).Scan(&runCount); err != nil || runCount != 0 {
		t.Fatalf("dry-run persisted %d runs: %v", runCount, err)
	}

	command := validEvidenceLineageBackfillCommand("source", 1)
	result, err := service.Backfill(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != "completed" || result.Run.ExaminedCount != 2 || result.Run.ReusedCount != 2 {
		t.Fatalf("apply run = %+v", result.Run)
	}
	var itemCount int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM evidence_lineage_migration_items WHERE run_id=$1 AND disposition='reused' AND target_resource_type='source_connection'`, result.Run.RunID).Scan(&itemCount); err != nil || itemCount != 2 {
		t.Fatalf("migration items = %d/%v", itemCount, err)
	}

	legacyResult, err := service.Backfill(ctx, validEvidenceLineageBackfillCommand("evidence_metadata", 10))
	if err != nil {
		t.Fatal(err)
	}
	if legacyResult.Run.SkippedCount != 1 || legacyResult.Run.CreatedCount != 0 {
		t.Fatalf("legacy evidence result = %+v", legacyResult.Run)
	}
	var reason string
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT reason_code FROM evidence_lineage_migration_items WHERE run_id=$1`, legacyResult.Run.RunID).Scan(&reason); err != nil || reason != "legacy_body_not_raw_evidence" {
		t.Fatalf("legacy evidence reason = %q/%v", reason, err)
	}
}

func TestEvidenceLineageBackfillRepositoryBlocksWhileProducerCanWrite(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO source_connections (source_type,name,endpoint) VALUES ('rss','blocked','https://blocked.example/feed')`); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO river_job (kind,args,state,unique_key) VALUES ('collect_source','{}','available',decode($1,'hex'))`, repeatMaintenanceSHA("9")); err != nil {
		t.Fatal(err)
	}
	service, err := operationsapplication.NewEvidenceLineageMaintenanceService(NewEvidenceLineageMaintenanceRepository(runtime))
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Backfill(ctx, validEvidenceLineageBackfillCommand("source", 10))
	if err == nil || len(result.Inspection.Blockers) == 0 || result.Inspection.Blockers[0] != "active_evidence_lineage_producers" {
		t.Fatalf("blocked result=%+v error=%v", result, err)
	}
}

func validEvidenceLineageBackfillCommand(phase string, batchSize int) operationsapplication.EvidenceLineageBackfillCommand {
	return operationsapplication.EvidenceLineageBackfillCommand{
		Phase: phase, BatchSize: batchSize, Apply: true, ConfirmNonEmpty: true,
		OperatorID: "operator-a", ReviewerID: "reviewer-b",
		BinarySHA256: repeatMaintenanceSHA("a"), SchemaSHA256: repeatMaintenanceSHA("b"),
		ConfigurationSHA256: repeatMaintenanceSHA("c"), BackupEvidenceSHA256: repeatMaintenanceSHA("d"),
		RehearsalEvidenceSHA256: repeatMaintenanceSHA("e"),
	}
}

func repeatMaintenanceSHA(value string) string {
	result := ""
	for range 64 {
		result += value
	}
	return result
}
