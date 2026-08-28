package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReportPublicationGateMatchesAC004002AndAC004003(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	contracts := map[string][]string{
		"backend/Makefile": {
			"report-publication-acceptance:",
			"TestReportPublicationRequiresExactMicroEventSentenceCitations",
			"TestMaliciousReportFixtureCannotCreateApprovedOrDownstreamFacts",
			"TestReportRevisionApprovalIsOptimisticAuditedAndRegenerationPreservesApprovedSnapshot",
			"TestReportRevisionConcurrentApprovalHasOneWinner",
			"content-security-policy.test.ts",
			"dashboard-reports-page.test.tsx",
		},
		"backend/test/_suite/internal/modules/report/infrastructure/postgres/report_evidence_integration_test.go": {
			"TestReportRepositoryFreezesExactSentenceCitationsAndPublishedVersion",
			"TestReportRevisionApprovalIsOptimisticAuditedAndRegenerationPreservesApprovedSnapshot",
			"TestReportRevisionConcurrentApprovalHasOneWinner",
			"TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState",
			"approved report body update succeeded",
			"approved revision changed after regeneration",
		},
		"backend/test/_suite/internal/modules/report/infrastructure/postgres/report_content_security_integration_test.go": {
			"TestMaliciousReportFixtureCannotCreateApprovedOrDownstreamFacts",
			"report.content_rejected",
			"report_content_unsafe",
			"business facts changed after repeated content attack",
		},
		"frontend/next.config.ts": {
			"Content-Security-Policy",
			"script-src-attr 'none'",
			"frame-ancestors 'none'",
		},
		"frontend/test/unit/lib/content-security-policy.test.ts": {
			"serves every page with a report-safe browser fallback",
			"script-src-attr 'none'",
			"not.toContain(\"'unsafe-eval'\")",
		},
		"frontend/test/unit/app/dashboard-reports-page.test.tsx": {
			"Evidence IDs：71",
			"document.querySelector(\"svg[onload]\")",
		},
		".github/workflows/ci.yml": {
			"http://127.0.0.1:8010/",
			"content-security-policy:",
			"script-src-attr 'none'",
			"frame-ancestors 'none'",
		},
	}
	for relative, required := range contracts {
		payload := readRepositoryFile(t, repository, relative)
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s is missing report publication evidence %q", relative, fragment)
			}
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/004-通知报告知识投影与检索计划.md")
	for _, fragment := range []string{
		"- [x] `CHK-004-G4-002`",
		"- [x] `CHK-004-G4-003`",
		"`make report-publication-acceptance`",
		"TestMaliciousReportFixtureCannotCreateApprovedOrDownstreamFacts",
		"TestReportRevisionConcurrentApprovalHasOneWinner",
	} {
		if !strings.Contains(plan, fragment) {
			t.Errorf("plan 004 is missing completed report publication evidence %q", fragment)
		}
	}
}
