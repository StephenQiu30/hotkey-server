package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestG5RCCandidateAssessmentGateAggregatesWithoutForgingReleaseApproval(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	tool := readRepositoryFile(t, repository, "backend/test/tools/rc-candidate-assessment/main.go")
	for _, fragment := range []string{
		`hotkey-rc-candidate-assessment-v1`,
		`automated_assessment_not_release_approval`,
		`"backend_static"`,
		`"backend_runtime"`,
		`"backend_vulnerability"`,
		`"worker_recovery"`,
		`"frontend"`,
		`"python_agent"`,
		`"compose"`,
		`"browser_business_flow"`,
		`-ACCEPTANCE-MISSING`,
		`UsedDeferredCapabilitiesAsEvidence: false`,
		`os.O_WRONLY|os.O_CREATE|os.O_EXCL`,
		`0o600`,
	} {
		if !strings.Contains(tool, fragment) {
			t.Errorf("RC assessment tool lost %q", fragment)
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	if !strings.Contains(makefile, "rc-candidate-assessment:") || !strings.Contains(makefile, "$(GO) run ./test/tools/rc-candidate-assessment") {
		t.Error("backend Makefile no longer exposes the RC candidate assessment")
	}

	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	for _, fragment := range []string{
		"Assess RC evidence and verify every required technical gate",
		"run: make rc-candidate-assessment",
		"Upload sanitized RC candidate assessment",
		"rc-candidate-assessment-${{ github.run_id }}-${{ github.run_attempt }}",
		"if-no-files-found: error",
	} {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("RC assessment CI gate is missing %q", fragment)
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/005-安全运维质量与交付计划.md")
	row := markdownChecklistRow(t, plan, "CHK-005-G5-005")
	if !strings.HasPrefix(row, "- [ ]") {
		t.Errorf("RC remains product-blocked but G5-005 was marked complete: %s", row)
	}

	acceptance := readRepositoryFile(t, repository, "docs/acceptance/005-安全运维质量与交付验收.md")
	for _, fragment := range []string{
		"bf8fd2d046b24eb52bce59a9676b6ea080b78587",
		"https://github.com/StephenQiu30/hotkey-server/actions/runs/33376192821",
		"https://github.com/StephenQiu30/hotkey-server/actions/runs/33378056399",
		"color-contrast",
		"4.47:1",
		"人工产品 UAT",
		"release_ready=false",
	} {
		if !strings.Contains(acceptance, fragment) {
			t.Errorf("005 Acceptance is missing current technical baseline fragment %q", fragment)
		}
	}
	for _, checkID := range []string{"CHK-005-G4-001", "CHK-005-G5-005"} {
		if row := markdownChecklistRow(t, plan, checkID); !strings.HasPrefix(row, "- [ ]") {
			t.Errorf("%s must remain open until product UAT and release evidence exist: %s", checkID, row)
		}
	}
}
