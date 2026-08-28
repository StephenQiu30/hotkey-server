package architecture_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func Test001PartialAcceptanceRecordsVerifiedBaselineAndHonestGaps(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	acceptancePath := oneNumberedDocument(t, filepath.Join(repository, "docs", "acceptance"), "001")
	acceptance := readRepositoryFile(t, repository, relativeDocumentPath(t, repository, acceptancePath))
	if status := frontmatterStatus(t, acceptancePath); status != "failed" {
		t.Errorf("001 remains partial and must use failed Acceptance status, got %q", status)
	}
	if !regexp.MustCompile(`(?m)^verified_revision: "[0-9a-f]{40}"$`).MatchString(acceptance) {
		t.Error("001 Acceptance must pin the exact verified Git revision")
	}
	for _, fragment := range []string{
		"EV-001-001",
		"EV-001-002",
		"EV-001-003",
		"EV-001-004",
		"EV-001-005",
		"EV-001-006",
		"EV-001-007",
		"EV-001-008",
		"EV-001-009",
		"EV-001-010",
		"EV-001-011",
		"EV-001-012",
		"https://github.com/StephenQiu30/hotkey-server/actions/runs/33216292257",
		"make agent-degradation-acceptance",
		"make report-publication-acceptance",
		"TestReportListCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert",
		"TestMicroEventEvidenceCursorIsSignedBoundExpiringAndSnapshotStable",
		"TestServiceCursorIsSignedBoundExpiringAndSnapshotStableAcrossOwners",
		"TestSearchRouteForwardsOpaqueCursorAndReturnsNextCursor",
		"TestMicroEventLexicalSearchUsesSnapshotKeysetOrdering",
		"TestJobRepositoryCursorIsSignedBoundExpiringAndSnapshotStable",
		"TestJobListForwardsOpaqueSubjectBoundCursorAndReturnsNextCursor",
		"TestGovernanceAuditCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert",
		"TestGovernanceAuditForwardsOpaqueSubjectBoundCursor",
		"operations_audit_list",
		"TestContentCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentChanges",
		"TestContentListStatementUsesDatabaseOrderingForEveryHotspotSort",
		"content_list",
		"TestMonitorListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert",
		"TestCodecRejectsNonCanonicalSignatureEncoding",
		"monitor_list",
		"TestP0UserListCursorsUseSignedExpiringCodec",
		"TestBrowserCIA11yAuditsWaitForVisualStateToSettle",
		"TestP0LexicalRecallUsesOnlyAuditablePostgresFTS",
		"TestRecoverVaultDocumentUsesOnlyProtectedHumanRegionSources",
		"TestForbiddenInfrastructureDetectorCatchesErroneousIntroductions",
	} {
		if !strings.Contains(acceptance, fragment) {
			t.Errorf("001 Acceptance is missing %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/001-HotKey产品需求分析与总体架构计划.md")
	completed := map[string]string{
		"CHK-001-G0-001": "EV-001-001",
		"CHK-001-G1-002": "EV-001-002",
		"CHK-001-G3-001": "EV-001-003",
		"CHK-001-G4-001": "EV-001-004",
		"CHK-001-G4-002": "EV-001-005",
	}
	for checkID, evidenceID := range completed {
		row := markdownChecklistRow(t, plan, checkID)
		if !strings.HasPrefix(row, "- [x]") || !strings.Contains(row, "`"+evidenceID+"`") {
			t.Errorf("%s must be completed only with %s: %s", checkID, evidenceID, row)
		}
		if !strings.Contains(acceptance, "`"+checkID+"`") || !strings.Contains(acceptance, "`"+evidenceID+"`") {
			t.Errorf("001 Acceptance does not map %s to %s", checkID, evidenceID)
		}
	}
	for _, checkID := range []string{
		"CHK-001-G1-001",
		"CHK-001-G2-001",
		"CHK-001-G3-002",
		"CHK-001-G3-003",
		"CHK-001-G3-004",
	} {
		if row := markdownChecklistRow(t, plan, checkID); !strings.HasPrefix(row, "- [ ]") {
			t.Errorf("%s must remain open until its full-scope evidence exists: %s", checkID, row)
		}
	}

	operations := readRepositoryFile(t, repository, "docs/operations/004-可观测性SLO与事件响应.md")
	if !strings.Contains(operations, "`EV-001-004`") || strings.Contains(operations, "`CHK-001-G4-001` 必须保持未勾选") {
		t.Error("the degradation runbook must point to recorded Acceptance evidence instead of a stale open-gate instruction")
	}
	index := readRepositoryFile(t, repository, "docs/acceptance/README.md")
	if !strings.Contains(index, "[部分验收](001-HotKey产品需求分析与总体架构验收.md)") || !strings.Contains(index, "failed") {
		t.Error("Acceptance index must expose the partial 001 result and its failed overall status")
	}
}
