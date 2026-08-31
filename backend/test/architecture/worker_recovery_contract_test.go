package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectionWorkerRecoveryGateCoversFourDurableCrashBoundaries(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	evidence := map[string]string{
		"backend/test/_suite/internal/platform/queue/worker_crash_recovery_integration_test.go":                                          "TestWorkerKillAfterClaimReclaimsLeaseAndAppliesSideEffectOnce",
		"backend/test/_suite/internal/modules/ingestion/application/worker_recovery_integration_test.go":                                 "TestIngestWorkerKillAfterMinIOWriteRecoversWithoutDuplicateSideEffects",
		"backend/test/_suite/internal/modules/source/infrastructure/postgres/evidence_worker_recovery_integration_test.go":               "TestEvidenceWorkerKillAfterObservationCommitRecoversOneDocumentJob",
		"backend/test/_suite/internal/modules/ingestion/infrastructure/postgres/document_projection_worker_recovery_integration_test.go": "TestDocumentProjectionWorkerKillAfterVaultWriteRecoversOneArtifact",
	}
	for path, testName := range evidence {
		content := readRepositoryFile(t, repository, path)
		if !strings.Contains(content, "func "+testName+"(") || !strings.Contains(content, "ReclaimStale") || !strings.Contains(content, "lease_expired") {
			t.Errorf("Worker recovery evidence %s in %s lost its crash/reclaim assertions", testName, path)
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	if !strings.Contains(makefile, "worker-recovery-acceptance: test-env minio-test-env") {
		t.Error("backend Makefile no longer exposes the four-point Worker recovery gate")
	}
	for _, testName := range evidence {
		if !strings.Contains(makefile, testName) {
			t.Errorf("Worker recovery gate no longer runs %s", testName)
		}
	}

	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	for _, fragment := range []string{
		"worker-recovery-acceptance:",
		"run: make worker-recovery-acceptance",
		"HOTKEY_TEST_MINIO_ENDPOINT",
		"minio/minio@sha256:",
		"needs: [backend-static-acceptance, backend-acceptance, backend-race-acceptance, backend-vulnerability-acceptance, worker-recovery-acceptance, frontend-acceptance, agent-acceptance, compose-acceptance, browser-smoke-acceptance]",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("CI Worker recovery gate is missing %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/002-监控来源采集与证据链计划.md")
	row := markdownChecklistRow(t, plan, "CHK-002-G4-004")
	if !strings.HasPrefix(row, "- [x]") {
		t.Errorf("four-point Worker recovery evidence exists but checklist is not complete: %s", row)
	}
	for _, testName := range evidence {
		if !strings.Contains(row, testName) {
			t.Errorf("CHK-002-G4-004 does not cite %s: %s", testName, row)
		}
	}
}

func TestG5RiverFaultRehearsalGateIsMandatory(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	rehearsal := readRepositoryFile(t, repository, "backend/test/_suite/internal/platform/queue/river_fault_rehearsal_integration_test.go")
	for _, fragment := range []string{
		`hotkey-river-fault-rehearsal-v1`,
		`TestRiverFaultRehearsalCoversEveryCrashRetryAndProviderLostAckBoundary`,
		`"before_claim"`,
		`"after_claim"`,
		`"business_transaction"`,
		`"before_completion_marker"`,
		`"provider_receipt"`,
		`"provider_unsupported"`,
		`bounded_at_least_once_with_fencing_idempotency_and_explicit_unknown`,
		`os.O_WRONLY|os.O_CREATE|os.O_EXCL`,
		`0o600`,
	} {
		if !strings.Contains(rehearsal, fragment) {
			t.Errorf("River fault rehearsal lost %q", fragment)
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	for _, fragment := range []string{
		"river-fault-rehearsal-acceptance: test-env",
		"TestRiverFaultRehearsalCoversEveryCrashRetryAndProviderLostAckBoundary",
		"TestRiverFaultRehearsalEvidenceWriterIsExclusivePrivateAndSanitized",
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("River fault Make gate is missing %q", fragment)
		}
	}

	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	for _, fragment := range []string{
		"Unified River fault and Provider lost-ack rehearsal acceptance",
		"run: make river-fault-rehearsal-acceptance",
		"Upload sanitized River fault rehearsal evidence",
		"river-fault-rehearsal-${{ github.run_id }}-${{ github.run_attempt }}",
		"if-no-files-found: error",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("River fault CI gate is missing %q", fragment)
		}
	}
}

func TestG5RepeatedRestoreRehearsalGateIsMandatory(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	rehearsal := readRepositoryFile(t, repository, "backend/test/tools/repeated-restore-drill/main.go")
	for _, fragment := range []string{
		`hotkey-repeated-restore-rehearsal-v1`,
		`[]string{"restore-a", "restore-b"}`,
		`IndependentComposeProject: true`,
		`NewVolumes: []string{"postgres_data", "minio_data", "vault_data"}`,
		`"missing_backup"`,
		`"corrupt_backup"`,
		`"schema_incompatible"`,
		`"reconciliation_mismatch"`,
		`StoppedBeforeCutover: true`,
		`ExistingTargetOverwritten: false`,
		`os.O_WRONLY|os.O_CREATE|os.O_EXCL`,
		`0o600`,
	} {
		if !strings.Contains(rehearsal, fragment) {
			t.Errorf("repeated restore rehearsal lost %q", fragment)
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	if !strings.Contains(makefile, "repeated-restore-rehearsal-acceptance:") ||
		!strings.Contains(makefile, "$(GO) run ./test/tools/repeated-restore-drill") {
		t.Error("backend Makefile no longer exposes the repeated restore rehearsal gate")
	}

	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	for _, fragment := range []string{
		"Repeated independent Compose restore and failure-stop acceptance",
		"make repeated-restore-rehearsal-acceptance",
		"Upload sanitized repeated restore evidence",
		"repeated-restore-${{ github.run_id }}-${{ github.run_attempt }}",
		"if-no-files-found: error",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("repeated restore CI gate is missing %q", fragment)
		}
	}
}

func markdownChecklistRow(t *testing.T, document, id string) string {
	t.Helper()
	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(line, "- [") && strings.Contains(line, "`"+id+"`") {
			return line
		}
	}
	t.Fatalf("checklist is missing %s", id)
	return ""
}
