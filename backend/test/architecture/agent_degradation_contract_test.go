package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentDegradationGateMatchesAC003003(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	evidence := map[string][]string{
		"backend/test/_suite/internal/modules/intelligence/application/service_integration_test.go": {
			"TestRunServiceKeepsPrimaryResultWhenPythonAgentIsUnavailable",
			"CodeAIModelUnavailable",
			"status != \"succeeded\"",
		},
		"backend/test/_suite/internal/modules/intelligence/infrastructure/agent/shadow_test.go": {
			"TestShadowRunnerMapsAgentFailureAndBoundsConcurrency",
			"observation.Result != \"dropped\"",
		},
		"backend/test/_suite/internal/modules/source/application/collection_service_integration_test.go": {
			"TestCollectionServiceProjectsThreeSourcePartialSuccessWithoutPersistingAggregateOutcome",
		},
		"backend/test/_suite/internal/modules/ingestion/application/content_family_test.go": {
			"TestContentFamilyServicePersistsOnlyFingerprintAndDecisionFacts",
		},
		"backend/test/_suite/internal/modules/event/application/event_heat_test.go": {
			"TestEventHeatServiceUsesActiveProfileWeightsAndStableSnapshot",
		},
		"backend/test/_suite/internal/modules/event/application/micro_event_governance_test.go": {
			"TestMicroEventGovernanceServiceBuildsStablePOJOCommand",
		},
		"backend/test/_suite/internal/modules/intelligence/infrastructure/jobs/recompute_test.go": {
			"TestAIRunRecomputeWorkerReactivatesOwningJobFromRunIDOnly",
			"secondCreated",
		},
	}
	for path, fragments := range evidence {
		content := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("AC-003-003 evidence %s is missing %q", path, fragment)
			}
		}
	}

	for _, path := range []string{
		"backend/internal/modules/source/application/collection_service.go",
		"backend/internal/modules/ingestion/application/content_family.go",
		"backend/internal/modules/event/application/event_heat.go",
		"backend/internal/modules/event/application/micro_event_governance.go",
	} {
		content := readRepositoryFile(t, repository, path)
		if strings.Contains(content, "modules/intelligence") || strings.Contains(content, "hotkey_agent") {
			t.Errorf("deterministic degradation path %s depends on the Agent runtime", path)
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	for _, fragment := range []string{
		"agent-degradation-acceptance:",
		"TestRunServiceKeepsPrimaryResultWhenPythonAgentIsUnavailable",
		"TestCollectionServiceProjectsThreeSourcePartialSuccessWithoutPersistingAggregateOutcome",
		"TestAIRunRecomputeWorkerReactivatesOwningJobFromRunIDOnly",
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("Agent degradation acceptance target is missing %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")
	row := markdownChecklistRow(t, plan, "CHK-003-G4-002")
	if !strings.HasPrefix(row, "- [x]") {
		t.Errorf("AC-003-003 evidence exists but checklist is not complete: %s", row)
	}
	for _, fragment := range []string{
		"TestRunServiceKeepsPrimaryResultWhenPythonAgentIsUnavailable",
		"TestContentFamilyServicePersistsOnlyFingerprintAndDecisionFacts",
		"TestAIRunRecomputeWorkerReactivatesOwningJobFromRunIDOnly",
		"make agent-degradation-acceptance",
	} {
		if !strings.Contains(row, fragment) {
			t.Errorf("CHK-003-G4-002 does not cite %q: %s", fragment, row)
		}
	}
}
