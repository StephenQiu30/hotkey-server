package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestG5PlannedRunbookDryRunStaysNonActivatingAndRunsInCI(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	operations := []string{
		"docs/operations/001-部署升级与回滚.md",
		"docs/operations/002-备份恢复与重建.md",
		"docs/operations/003-来源授权预算与故障处置.md",
		"docs/operations/004-可观测性SLO与事件响应.md",
		"docs/operations/005-保留删除与撤权处置.md",
		"docs/operations/006-密钥轮换与泄漏响应.md",
	}
	for _, path := range operations {
		if status := frontmatterStatus(t, filepath.Join(repository, path)); status != "planned" {
			t.Errorf("%s status = %q, want planned until G6 approval", path, status)
		}
	}

	tool := readRepositoryFile(t, repository, "backend/test/tools/runbook-dry-run/main.go")
	for _, marker := range []string{
		`reportVersion          = "hotkey-planned-runbook-dry-run-v1"`,
		`"contract_and_repository_entrypoint_resolution_without_operational_side_effects"`,
		`ActivationEligible: false`,
		`os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600`,
		`PendingG6Activation: []string{"001", "002", "003", "004", "005", "006"}`,
	} {
		if !strings.Contains(tool, marker) {
			t.Errorf("Runbook dry-run tool is missing non-activating evidence marker %q", marker)
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	if !strings.Contains(makefile, "runbook-dry-run-acceptance:") || !strings.Contains(makefile, "$(GO) run ./test/tools/runbook-dry-run") {
		t.Error("backend Makefile does not expose the planned Runbook dry-run gate")
	}
	validator := readRepositoryFile(t, repository, "backend/test/tools/validate-architecture.sh")
	if !strings.Contains(validator, "go run ./test/runner test ./test/architecture") {
		t.Error("architecture validation bypasses the centralized test suite runner")
	}
	workflow := readRepositoryFile(t, repository, ".github/workflows/ci.yml")
	for _, marker := range []string{
		"Planned Runbook dry-run acceptance",
		"HOTKEY_RUNBOOK_DRY_RUN_PRODUCTION_EGRESS_DISABLED: \"true\"",
		"runbook-dry-run-${{ github.run_id }}-${{ github.run_attempt }}",
	} {
		if !strings.Contains(workflow, marker) {
			t.Errorf("CI does not preserve Runbook dry-run evidence marker %q", marker)
		}
	}
}
