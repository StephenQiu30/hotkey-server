package architecture_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func Test002PartialAcceptanceRecordsPassedEvidenceAndHonestReleaseGaps(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	acceptancePath := "docs/acceptance/002-监控来源采集与证据链验收.md"
	acceptance := readRepositoryFile(t, repository, acceptancePath)
	if status := frontmatterStatus(t, filepath.Join(repository, acceptancePath)); status != "failed" {
		t.Fatalf("002 partial Acceptance must remain failed, got %q", status)
	}
	if !regexp.MustCompile(`(?m)^verified_revision: "6ff813bc8e90bd673d24d8d50b1ef076d5a70a04"$`).MatchString(acceptance) {
		t.Errorf("002 Acceptance must pin the exact verified implementation revision")
	}

	plan := readRepositoryFile(t, repository, "docs/plans/002-监控来源采集与证据链计划.md")
	completed := map[string]string{
		"CHK-002-G3-001": "EV-002-001",
		"CHK-002-G3-003": "EV-002-002",
		"CHK-002-G4-001": "EV-002-003",
		"CHK-002-G4-002": "EV-002-004",
		"CHK-002-G4-003": "EV-002-005",
		"CHK-002-G4-004": "EV-002-006",
		"CHK-002-G4-005": "EV-002-007",
		"CHK-002-G4-006": "EV-002-002",
	}
	for checkID, evidenceID := range completed {
		row := markdownChecklistRow(t, plan, checkID)
		if !strings.HasPrefix(row, "- [x]") {
			t.Errorf("%s must remain completed: %s", checkID, row)
		}
		if !strings.Contains(acceptance, "`"+checkID+"`") || !strings.Contains(acceptance, "`"+evidenceID+"`") ||
			!strings.Contains(plan, "`"+checkID+"`") || !strings.Contains(plan, "`"+evidenceID+"`") {
			t.Errorf("%s is not traceable through %s in Plan and Acceptance", checkID, evidenceID)
		}
	}

	open := map[string]string{
		"CHK-002-G3-002": "EV-002-011",
		"CHK-002-G5-001": "EV-002-008",
	}
	for checkID, evidenceID := range open {
		row := markdownChecklistRow(t, plan, checkID)
		if !strings.HasPrefix(row, "- [ ]") || !strings.Contains(row, evidenceID) {
			t.Errorf("%s must remain open and cite partial %s: %s", checkID, evidenceID, row)
		}
	}

	for _, fragment := range []string{
		"`AC-002-002` | partial",
		"`AC-002-008` | partial",
		"TestCollectionFetchRightsDenialStopsBeforeConnectorResolutionAndPersistsSanitizedAudit",
		"TestConnectorRegistryDefersManagedCredentialDecryptionUntilTheRequestBoundary",
		"TestXEndpointPolicyRejectsUnsafeResolutionBeforeCredentialBudgetOrDial",
		"`EV-002-010`",
		"TestGovernanceAuditQueryCoversFiveSourceManagementCategoriesWithoutSyntheticSecrets",
		"TestSourceServiceAuditsBudgetConfigurationChangeWithoutPersistingConfiguration",
		"make source-management-audit-acceptance",
		"`EV-002-011`",
		"TestCollectionAdmissionChecksBudgetAndRateLimitAfterCredentialStatus",
		"TestCollectionHealthAdmissionDenialStopsBeforeConnectorResolutionAndPersistsSafeAudit",
		"TestExternalRequestBudgetEnforcesPerMinuteRateLimitAtomicallyWithoutConsumingDailyBudget",
		"make source-admission-matrix-acceptance",
		"只剩三来源真实授权冒烟",
		"不得用 Fixture、模拟响应或本机候选容量替代真实授权证据",
		"33253811195",
	} {
		if !strings.Contains(acceptance, fragment) {
			t.Errorf("002 Acceptance is missing %q", fragment)
		}
	}

	index := readRepositoryFile(t, repository, "docs/acceptance/README.md")
	if !strings.Contains(index, "[部分验收](002-监控来源采集与证据链验收.md)") || !strings.Contains(index, "002 | 监控来源采集与证据链 | in_progress") {
		t.Error("Acceptance index must expose the partial 002 result")
	}
}
