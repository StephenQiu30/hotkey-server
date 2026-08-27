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

	unavailable := markdownTableRow(t, matrix, "DEG-001-CODEX-UNAVAILABLE")
	for _, evidence := range []string{
		"TestRunServicePersistsTerminalOperationalFailureAsPendingAnalysis",
		"TestAutomaticClaimEvidenceServiceLeavesOperationalModelFailuresPendingWithoutBusinessWrites",
		"TestCollectionServiceFetchesOnceAndDurablyReconcilesEveryTarget",
		"TestContentFamilyServicePersistsOnlyFingerprintAndDecisionFacts",
		"TestEventHeatServiceUsesActiveProfileWeightsAndStableSnapshot",
	} {
		if !strings.Contains(unavailable, "`"+evidence+"`") {
			t.Errorf("Codex unavailable row does not name %s: %s", evidence, unavailable)
		}
	}
	if !strings.Contains(unavailable, "`partial`") || !strings.Contains(unavailable, "TASK-003-S02-T03") {
		t.Errorf("Codex unavailable must keep administrator backfill as an explicit next task: %s", unavailable)
	}

	unauthorized := markdownTableRow(t, matrix, "DEG-001-CODEX-UNAUTHORIZED-SUGGESTION")
	for _, evidence := range []string{
		"TestRunServiceNeverPersistsOrReusesForgedEvidenceAndRepairConsumesNewAttempt",
		"TestStructuredOutputPolicyRejectsEvidenceOutsideExactInputWhitelist",
	} {
		if !strings.Contains(unauthorized, "`"+evidence+"`") {
			t.Errorf("Codex unauthorized suggestion row does not name %s: %s", evidence, unauthorized)
		}
	}
	if !strings.Contains(unauthorized, "`verified`") {
		t.Errorf("Codex unauthorized suggestion must retain verified evidence: %s", unauthorized)
	}
	for testName, path := range map[string]string{
		"TestRunServicePersistsTerminalOperationalFailureAsPendingAnalysis":                           "backend/test/_suite/internal/modules/intelligence/application/service_integration_test.go",
		"TestAutomaticClaimEvidenceServiceLeavesOperationalModelFailuresPendingWithoutBusinessWrites": "backend/test/_suite/internal/modules/event/application/automatic_claim_evidence_test.go",
		"TestCollectionServiceFetchesOnceAndDurablyReconcilesEveryTarget":                             "backend/test/_suite/internal/modules/source/application/collection_service_integration_test.go",
		"TestContentFamilyServicePersistsOnlyFingerprintAndDecisionFacts":                             "backend/test/_suite/internal/modules/ingestion/application/content_family_test.go",
		"TestEventHeatServiceUsesActiveProfileWeightsAndStableSnapshot":                               "backend/test/_suite/internal/modules/event/application/event_heat_test.go",
		"TestRunServiceNeverPersistsOrReusesForgedEvidenceAndRepairConsumesNewAttempt":                "backend/test/_suite/internal/modules/intelligence/application/service_integration_test.go",
		"TestStructuredOutputPolicyRejectsEvidenceOutsideExactInputWhitelist":                         "backend/test/_suite/internal/modules/intelligence/application/structured_output_policy_test.go",
	} {
		if source := readRepositoryFile(t, repository, path); !strings.Contains(source, "func "+testName+"(") {
			t.Errorf("Codex degradation evidence %s no longer exists in %s", testName, path)
		}
	}
	for _, path := range []string{
		"backend/internal/modules/source/application/collection_service.go",
		"backend/internal/modules/ingestion/application/content_family.go",
		"backend/internal/modules/event/application/event_heat.go",
	} {
		source := readRepositoryFile(t, repository, path)
		if strings.Contains(source, "modules/intelligence") || strings.Contains(source, "Codex") {
			t.Errorf("deterministic degradation path %s gained an intelligence runtime dependency", path)
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
