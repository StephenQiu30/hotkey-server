package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSourcePermissionAndFiveStateGateMatchesAC002007(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	requiredEvidence := map[string][]string{
		"backend/test/_suite/internal/modules/monitor/application/service_integration_test.go": {
			"TestAnalystCanManageOnlyAnOwnedMonitor",
		},
		"backend/test/_suite/internal/modules/source/transport/http/handler_test.go": {
			"TestSourceRoutesRequireAdminForManagement",
			"TestSourceReadRoutesUseRoleDependentSafeUnion",
			"httptransport.RoleAnalyst",
		},
		"backend/test/_suite/internal/modules/source/transport/http/collection_handler_test.go": {
			"TestCollectionAdminRoutesEnforceRolesAndExposeOnlySafeRunFacts",
			"analyst manual",
			"analyst list denied",
		},
		"backend/test/_suite/internal/modules/ingestion/transport/http/handler_test.go": {
			"TestContentDocumentRouteAllowsAuthenticatedRolesAndReturnsSafeProjection",
			"analyst cannot delete",
			`"object_key"`,
		},
		"frontend/test/unit/app/dashboard-settings-page.test.tsx": {
			"shows an accessible loading state while the monitor request is pending",
			"shows an explicit empty state without management actions for a viewer",
			"shows a retryable load error instead of an empty monitor list",
			"shows a dedicated forbidden state without presenting a retry action",
			"lets an analyst manage only monitors they own",
		},
		"frontend/test/unit/app/dashboard-sources-page.test.tsx": {
			"loads the safe source directory without management actions for %s",
			"announces loading before showing an explicit empty source state",
			"shows a retryable source error separately from an empty result",
			"shows a dedicated forbidden source state without retrying",
		},
		"frontend/test/unit/app/dashboard-contents-page.test.tsx": {
			"renders server statistics and the same flat hotspot card as instant search",
			"announces the hotspot loading state",
			"shows request failures separately from a valid empty result and retries",
			"shows a dedicated forbidden hotspot state without a retry action",
		},
		"frontend/test/unit/lib/openapi-generation.test.ts": {
			"provides a deterministic contract check around the official openapi2ts CLI",
			"keeps application code on the generated server client only",
		},
	}
	for path, fragments := range requiredEvidence {
		content := readRepositoryFile(t, repository, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("AC-002-007 evidence %s is missing %q", path, fragment)
			}
		}
	}

	access := readRepositoryFile(t, repository, "frontend/src/lib/dashboardAccess.ts")
	if strings.Contains(access, `"/dashboard/sources"`) {
		t.Error("the safe source directory must not be restricted to Admin while its read API supports every product role")
	}
	sourcePage := readRepositoryFile(t, repository, "frontend/src/app/dashboard/sources/page.tsx")
	for _, fragment := range []string{
		`canManage && source.endpoint`,
		`aria-label="正在加载来源"`,
		`aria-label="来源访问权限不足"`,
		`@/services/hotkey/hotkey-server/sources`,
	} {
		if !strings.Contains(sourcePage, fragment) {
			t.Errorf("source page lost safe role/five-state behavior %q", fragment)
		}
	}
	if strings.Contains(sourcePage, "if (!canManage) return null") {
		t.Error("source page silently hides the safe read projection from non-Admin roles")
	}
	contentPage := readRepositoryFile(t, repository, "frontend/src/app/dashboard/contents/page.tsx")
	for _, fragment := range []string{
		`aria-label="正在加载信号"`,
		`aria-label="信号流访问权限不足"`,
		`@/services/hotkey/hotkey-server/hotspots`,
	} {
		if !strings.Contains(contentPage, fragment) {
			t.Errorf("content page lost generated-client/five-state behavior %q", fragment)
		}
	}

	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	for _, fragment := range []string{"npm run openapi:check", "npm run test:unit"} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("frontend CI no longer enforces %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/002-监控来源采集与证据链计划.md")
	row := markdownChecklistRow(t, plan, "CHK-002-G4-005")
	if !strings.HasPrefix(row, "- [x]") {
		t.Errorf("AC-002-007 evidence exists but checklist is not complete: %s", row)
	}
	for _, testName := range []string{
		"TestSourceReadRoutesUseRoleDependentSafeUnion",
		"TestContentDocumentRouteAllowsAuthenticatedRolesAndReturnsSafeProjection",
		"dashboard-settings-page.test.tsx",
		"dashboard-sources-page.test.tsx",
		"dashboard-contents-page.test.tsx",
		"openapi-generation.test.ts",
	} {
		if !strings.Contains(row, testName) {
			t.Errorf("CHK-002-G4-005 does not cite %s: %s", testName, row)
		}
	}
}
