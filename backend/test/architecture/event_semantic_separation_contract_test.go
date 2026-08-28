package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEventSemanticSeparationGateMatchesAC003005(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	evidence := map[string][]string{
		"backend/test/_suite/internal/modules/event/domain/claim_evidence_test.go": {
			"TestRelevanceHeatClaimEvidenceAndEventEvidenceStateRemainIndependent",
			"changedRelevance",
			"changedHeatInput",
			"changedClaimEvidence",
		},
		"backend/test/_suite/internal/modules/report/infrastructure/postgres/report_evidence_integration_test.go": {
			"TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState",
			"missingCitation",
			"hashMismatch",
			"textMismatch",
		},
		"backend/test/_suite/internal/modules/event/transport/http/micro_event_handler_test.go": {
			"TestMicroEventResponseKeepsRelevanceHeatAndEvidenceStateSeparate",
			"RelevanceScore",
			"LatestHeat",
			"EvidenceState",
		},
		"frontend/test/unit/app/dashboard-events-page.test.tsx": {
			`relevance_score: 0.34`,
			`heat_score: 82.45`,
			`state: "multiple_origins"`,
			`screen.getByText("34.0%")`,
		},
		"frontend/test/unit/app/dashboard-reports-page.test.tsx": {
			`heat_score: 88`,
			`claim_evidence_version_ids: [71]`,
			`screen.getByText("Evidence IDs：71")`,
		},
	}
	for path, fragments := range evidence {
		content := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("AC-003-005 evidence %s is missing %q", path, fragment)
			}
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	for _, fragment := range []string{
		"event-semantic-separation-acceptance: test-env",
		"TestRelevanceHeatClaimEvidenceAndEventEvidenceStateRemainIndependent",
		"TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState",
		"TestMicroEventResponseKeepsRelevanceHeatAndEvidenceStateSeparate",
		"dashboard-events-page.test.tsx dashboard-reports-page.test.tsx",
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("event semantic separation gate lost %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")
	row := markdownChecklistRow(t, plan, "CHK-003-G4-004")
	if !strings.HasPrefix(row, "- [x]") {
		t.Errorf("AC-003-005 evidence exists but checklist is not complete: %s", row)
	}
	for _, fragment := range []string{
		"TestRelevanceHeatClaimEvidenceAndEventEvidenceStateRemainIndependent",
		"TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState",
		"TestMicroEventResponseKeepsRelevanceHeatAndEvidenceStateSeparate",
		"dashboard-events-page.test.tsx",
		"dashboard-reports-page.test.tsx",
		"make event-semantic-separation-acceptance",
		"TestEventSemanticSeparationGateMatchesAC003005",
	} {
		if !strings.Contains(row, fragment) {
			t.Errorf("CHK-003-G4-004 does not cite %q: %s", fragment, row)
		}
	}
}
