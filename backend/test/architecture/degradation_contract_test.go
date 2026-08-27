package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestS03DegradationMatrixNamesExecutableEvidenceAndHonestGaps(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	matrix := readRepositoryFile(t, repository, "docs/operations/004-可观测性SLO与事件响应.md")

	evidence := map[string]struct {
		path string
		test string
	}{
		"DEG-001-SINGLE-SOURCE": {
			path: "backend/test/_suite/test/integration/pipeline_test.go",
			test: "TestRSSHNPipelineRecovery",
		},
		"DEG-001-REDIS-TRANSIENT": {
			path: "backend/test/_suite/internal/modules/identity/infrastructure/redis/verification_store_test.go",
			test: "TestVerificationStoreReportsUnavailableForClosedRedisClientOnEveryOperation",
		},
		"DEG-001-MINIO-WRITE": {
			path: "backend/test/_suite/internal/modules/source/application/collection_evidence_integration_test.go",
			test: "TestCollectionServiceFailsRetryablyWhenAuthorizedRawArchiveStorageFails",
		},
		"DEG-001-VAULT-UNWRITABLE": {
			path: "backend/test/_suite/internal/modules/ingestion/infrastructure/postgres/derived_artifact_repository_integration_test.go",
			test: "TestDerivedArtifactSagaMarksRealVaultFailureWithoutQuarantine",
		},
	}
	for fixture, contract := range evidence {
		if !strings.Contains(matrix, "`"+fixture+"`") || !strings.Contains(matrix, "`"+contract.test+"`") {
			t.Errorf("degradation matrix does not map %s to %s", fixture, contract.test)
		}
		testSource := readRepositoryFile(t, repository, contract.path)
		if !strings.Contains(testSource, "func "+contract.test+"(") {
			t.Errorf("degradation evidence %s no longer exists in %s", contract.test, contract.path)
		}
	}

	for _, pendingFixture := range []string{
		"DEG-001-CODEX-UNAVAILABLE",
		"DEG-001-CODEX-UNAUTHORIZED-SUGGESTION",
	} {
		row := markdownTableRow(t, matrix, pendingFixture)
		if !strings.Contains(row, "`PENDING-003-S01/S02`") || !strings.Contains(row, "`pending`") {
			t.Errorf("%s must stay an explicit 003 implementation gap, got %s", pendingFixture, row)
		}
	}

	redisRow := markdownTableRow(t, matrix, "DEG-001-REDIS-TRANSIENT")
	if !strings.Contains(redisRow, "`partial`") || !strings.Contains(redisRow, "尚需真实断开/恢复往返测试") {
		t.Errorf("Redis degradation must not be presented as fully verified: %s", redisRow)
	}
	if !strings.Contains(matrix, "`CHK-001-G4-001` 必须保持未勾选") {
		t.Error("degradation matrix no longer protects the incomplete G4 checklist state")
	}
}

func markdownTableRow(t *testing.T, document, id string) string {
	t.Helper()
	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(line, "|") && strings.Contains(line, "`"+id+"`") {
			return line
		}
	}
	t.Fatalf("markdown table is missing %s", id)
	return ""
}
