package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMonitorPublicationGateMatchesAC002001(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	requiredEvidence := map[string][]string{
		"backend/test/_suite/internal/modules/monitor/domain/monitor_test.go": {
			"TestMonitorStateTransitionsFollowPublishedStateMachine",
		},
		"backend/test/_suite/internal/modules/monitor/domain/intent_version_test.go": {
			"TestPublishedIntentVersionKeepsImmutableApprovedSnapshot",
			"published version candidates were mutated through accessor",
		},
		"backend/test/_suite/internal/modules/monitor/application/service_integration_test.go": {
			"TestMonitorServicePublishesImmutableConfigurationAndCoordinatesSourceLifecycle",
			"TestMonitorServiceFirstDraftAndPublishConcurrencyAndAuditRollback",
			"preview wrote persistent facts",
			"mutating published child succeeded",
		},
		"backend/test/_suite/internal/modules/monitor/application/intent_compiler_test.go": {
			"TestIntentCompilerPersistsExactPreviewProfileWithApprovedFactsOnly",
			"TestCompiledIntentProfileHashIsStableAcrossEquivalentPreviewRuns",
		},
		"backend/test/_suite/internal/modules/monitor/application/intent_publication_test.go": {
			"TestIntentPublicationStagesExactPreviewFactsAndCompletesPublishedProfile",
		},
		"backend/test/_suite/internal/modules/monitor/infrastructure/postgres/compiled_intent_profile_repository_integration_test.go": {
			"TestIntentRepositoryPersistsExactPreviewCompiledProfileIdempotently",
			"same preview owner different profile error",
		},
		"backend/test/_suite/internal/modules/monitor/infrastructure/postgres/intent_publication_repository_integration_test.go": {
			"TestIntentPublicationRepositoryPromotesSuccessfulExactPreviewInsidePublishTransaction",
		},
		"backend/test/_suite/internal/modules/monitor/infrastructure/postgres/collection_target_reader_test.go": {
			"TestCollectionSchedulerSkipsPublishedMonitorWithoutReadyCompiledProfile",
			"TestPublishedCollectionTargetReaderPrefersExactCompiledIntentOverLegacyRules",
			"TestCollectionSchedulerEnqueuesDueSourceWithoutWritingCollectionFacts",
			"want duplicate suppression",
		},
	}
	for path, fragments := range requiredEvidence {
		content := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("AC-002-001 evidence %s is missing %q", path, fragment)
			}
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	for _, fragment := range []string{
		"monitor-publication-acceptance:",
		"TestMonitorServicePublishesImmutableConfigurationAndCoordinatesSourceLifecycle",
		"TestCompiledIntentProfileHashIsStableAcrossEquivalentPreviewRuns",
		"TestIntentRepositoryPersistsExactPreviewCompiledProfileIdempotently",
		"TestCollectionSchedulerSkipsPublishedMonitorWithoutReadyCompiledProfile",
		"TestCollectionSchedulerEnqueuesDueSourceWithoutWritingCollectionFacts",
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("monitor publication acceptance target is missing %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/002-监控来源采集与证据链计划.md")
	row := markdownChecklistRow(t, plan, "CHK-002-G3-001")
	if !strings.HasPrefix(row, "- [x]") {
		t.Errorf("AC-002-001 evidence exists but checklist is not complete: %s", row)
	}
	for _, fragment := range []string{
		"TestMonitorServiceFirstDraftAndPublishConcurrencyAndAuditRollback",
		"TestCompiledIntentProfileHashIsStableAcrossEquivalentPreviewRuns",
		"TestCollectionSchedulerSkipsPublishedMonitorWithoutReadyCompiledProfile",
		"make monitor-publication-acceptance",
	} {
		if !strings.Contains(row, fragment) {
			t.Errorf("CHK-002-G3-001 does not cite %q: %s", fragment, row)
		}
	}
}

func TestSimpleMonitorBrowserAcceptancePublishesReadyCompiledProfile(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	evidence := map[string][]string{
		".github/workflows/ci.yml": {
			`agent-browser open "$HOTKEY_BROWSER_WEB_ORIGIN/dashboard/settings"`,
			`agent-browser find role button click --name "创建并启用" --exact`,
			`agent-browser wait --text "监控已创建并启用"`,
		},
		"backend/test/fixtures/browser-acceptance/seed.sql": {
			"Browser Acceptance RSS",
			"https://feeds.example.test/browser-acceptance",
		},
		"backend/test/fixtures/browser-acceptance/assert-business-flow.sql": {
			"published_profile.purpose = 'published'",
			"published_profile.status = 'ready'",
			"preview_profile.id = published_profile.source_preview_compiled_profile_id",
			"preview_profile.intent_revision_id = published_profile.intent_revision_id",
			"preview_run.status = 'succeeded'",
			"browser-created monitor is not bound to a ready published compiled profile",
		},
	}
	for path, fragments := range evidence {
		content := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("simple Monitor browser acceptance %s is missing %q", path, fragment)
			}
		}
	}
}
