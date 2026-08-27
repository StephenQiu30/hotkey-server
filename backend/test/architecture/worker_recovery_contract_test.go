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
		"needs: [backend-static-acceptance, backend-acceptance, backend-vulnerability-acceptance, worker-recovery-acceptance",
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
