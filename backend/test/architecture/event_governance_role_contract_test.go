package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEventGovernanceFourRoleAndFiveStateGateMatchesAC003008(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	backendTest := readRepositoryFile(t, repository, "backend/test/_suite/internal/modules/event/transport/http/micro_event_handler_test.go")
	for _, fragment := range []string{
		"TestMicroEventRoutesEnforceFourRoleReadAndGovernanceBoundary",
		"httptransport.RoleViewer",
		"httptransport.RoleAnalyst",
		"httptransport.RoleEditor",
		"httptransport.RoleAdmin",
		"wantGovernStatus: http.StatusForbidden",
		"wantGovernStatus: http.StatusOK",
		"governance writes",
	} {
		if !strings.Contains(backendTest, fragment) {
			t.Errorf("event governance role matrix lost %q", fragment)
		}
	}

	frontendTest := readRepositoryFile(t, repository, "frontend/test/unit/app/dashboard-event-governance-page.test.tsx")
	for _, fragment := range []string{
		`renders the normal governance projection from generated clients`,
		`keeps a dedicated loading state while durable facts are pending`,
		`distinguishes an event with no current governance facts`,
		`shows a retryable load error and recovers without stale facts`,
		`keeps a server forbidden response as an explicit page state`,
		`uses exact-version headers and stops on a concurrent update`,
		`[UserRole.Viewer, "当前账号为只读角色"]`,
		`[UserRole.Analyst, "Analyst 在事件治理中为只读角色"]`,
		`it.each([UserRole.Editor, UserRole.Admin])`,
	} {
		if !strings.Contains(frontendTest, fragment) {
			t.Errorf("event governance page acceptance lost %q", fragment)
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	for _, fragment := range []string{
		"event-governance-role-acceptance:",
		"TestMicroEventRoutesEnforceFourRoleReadAndGovernanceBoundary",
		"dashboard-event-governance-page.test.tsx",
	} {
		if !strings.Contains(makefile, fragment) {
			t.Errorf("event governance acceptance gate lost %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")
	row := markdownChecklistRow(t, plan, "CHK-003-G4-007")
	if !strings.HasPrefix(row, "- [x]") {
		t.Errorf("AC-003-008 evidence exists but checklist is not complete: %s", row)
	}
	for _, evidence := range []string{
		"TestMicroEventRoutesEnforceFourRoleReadAndGovernanceBoundary",
		"dashboard-event-governance-page.test.tsx",
		"TestEventGovernanceFourRoleAndFiveStateGateMatchesAC003008",
	} {
		if !strings.Contains(row, evidence) {
			t.Errorf("CHK-003-G4-007 does not cite %s: %s", evidence, row)
		}
	}
}
