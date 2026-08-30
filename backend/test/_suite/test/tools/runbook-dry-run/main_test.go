package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectRunbookAcceptsCompletePlannedContract(t *testing.T) {
	repository := t.TempDir()
	path := filepath.Join(repository, "docs", "operations", "001.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
status: planned
---
# Runbook
## 参数与隔离目标
使用独立 Compose Project，生产出口禁用。
## 执行命令与预期输出
` + "```bash\ndocker compose config --quiet\n```" + `
预期输出为空，退出码为 0。
## 停止与回滚
失败立即停止；执行回滚后重新验证。
## 审计证据
记录 revision、操作者、复核人和差异。
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "docker-compose.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := inspectRunbook(repository, runbookContract{
		ID: "001", Path: "docs/operations/001.md", CommandMarkers: []string{"docker compose config"},
	})
	if len(result.Differences) != 0 {
		t.Fatalf("differences = %v, want none", result.Differences)
	}
	if result.Status != "planned" || result.ActivationEligible || !result.ParametersPresent ||
		!result.IsolationPresent || !result.CopyableCommandsPresent || !result.ExpectedOutputPresent ||
		!result.StopPointsPresent || !result.RollbackPresent || !result.AuditPresent || !result.EntrypointsVerified {
		t.Fatalf("unexpected runbook result: %+v", result)
	}
}

func TestInspectRunbookRejectsActiveAndMissingRollback(t *testing.T) {
	repository := t.TempDir()
	path := filepath.Join(repository, "docs", "operations", "003.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
status: active
---
# Runbook
## 参数
记录参数。
## 隔离命令与预期输出
` + "```bash\nmake source-live-smoke\n```" + `
成功判据明确。
## 停止条件
失败停止。
## 审计证据
记录审计。
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	result := inspectRunbook(repository, runbookContract{
		ID: "003", Path: "docs/operations/003.md", CommandMarkers: []string{"make source-live-smoke"},
	})
	joined := strings.Join(result.Differences, ",")
	for _, want := range []string{"status_must_remain_planned", "rollback_or_recovery_missing"} {
		if !strings.Contains(joined, want) {
			t.Errorf("differences %q do not contain %q", joined, want)
		}
	}
	if result.ActivationEligible {
		t.Fatal("G5 dry-run must never activate a runbook")
	}
}

func TestWriteExclusiveReportIsPrivateAndRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runbook.json")
	value := report{
		Version: reportVersion, Status: "verified", Approval: "required",
		Environment: "isolated-ci", Hardware: "fixed-runner", GitRevision: strings.Repeat("a", 40),
		Isolated: true, ProductionEgressDisabled: true,
		DryRunScope: "contract_and_repository_entrypoint_resolution_without_operational_side_effects", ActivationEligible: false,
		PendingG6Activation: []string{"001"}, Runbooks: []runbookResult{}, Differences: []string{},
	}
	if err := writeExclusiveReport(path, value); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	if err := writeExclusiveReport(path, value); err == nil {
		t.Fatal("expected exclusive-create failure")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"token", "password", repositorySecretCanary} {
		if strings.Contains(strings.ToLower(string(payload)), strings.ToLower(forbidden)) {
			t.Fatalf("report contains forbidden value %q", forbidden)
		}
	}
}
