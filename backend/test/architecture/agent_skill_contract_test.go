package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentSkillContractGateMatchesAC003002(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	requiredEvidence := map[string][]string{
		"agent/src/hotkey_agent/skills.py": {
			"input_schema_sha256",
			"Draft202012Validator",
			"validate_skill_request",
			"validate_analysis_response",
		},
		"agent/tests/test_api.py": {
			"test_every_structured_skill_validates_canonical_input_and_output_schema",
			"test_structured_analysis_rejects_forged_input_schema_and_invalid_input",
			"test_structured_analysis_rejects_invalid_or_forged_analyzer_output",
		},
		"backend/internal/modules/intelligence/application/run_service.go": {
			"InputSchema: contract.InputSchema",
			"ValidateOutput",
			"validateStructuredOutputPolicy",
		},
		"backend/test/_suite/internal/modules/intelligence/infrastructure/agent/client_test.go": {
			"TestClientMapsStatusTimeoutAndMalformedResponsesToStableErrors",
			"TestClientRejectsIdentityAndEvidenceForgery",
			"TestClientAdaptsStructuredRequestsToVersionedAgentPayload",
		},
		"backend/test/_suite/internal/modules/intelligence/application/prompt_injection_integration_test.go": {
			"TestPromptInjectionFixtureCannotChangeEvidenceOrOutputContract",
			"succeededRuns != 0",
		},
		"backend/test/_suite/internal/modules/event/application/automatic_claim_evidence_test.go": {
			"TestAutomaticClaimEvidenceServiceLeavesOperationalModelFailuresPendingWithoutBusinessWrites",
			"facts.committed.ClaimHash != \"\"",
		},
	}
	for path, fragments := range requiredEvidence {
		content := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("AC-003-002 evidence %s is missing %q", path, fragment)
			}
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	for _, fragment := range []string{
		"agent-skill-contract-acceptance:",
		"cd ../agent && uv run pytest",
		"TestClientRejectsIdentityAndEvidenceForgery",
		"TestPromptInjectionFixtureCannotChangeEvidenceOrOutputContract",
		"TestAutomaticClaimEvidenceServiceLeavesOperationalModelFailuresPendingWithoutBusinessWrites",
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("Agent Skill contract acceptance target is missing %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")
	row := markdownChecklistRow(t, plan, "CHK-003-G4-001")
	if !strings.HasPrefix(row, "- [x]") {
		t.Errorf("AC-003-002 evidence exists but checklist is not complete: %s", row)
	}
	for _, fragment := range []string{
		"test_every_structured_skill_validates_canonical_input_and_output_schema",
		"TestClientRejectsIdentityAndEvidenceForgery",
		"TestPromptInjectionFixtureCannotChangeEvidenceOrOutputContract",
		"make agent-skill-contract-acceptance",
	} {
		if !strings.Contains(row, fragment) {
			t.Errorf("CHK-003-G4-001 does not cite %q: %s", fragment, row)
		}
	}
}
