package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonAgentCapacityAndIsolationGateMatchesAC003001(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	tool := readRepositoryFile(t, repository, "agent/tests/tools/capacity_baseline.py")
	for _, fragment := range []string{
		`"relevance"`,
		`"claim_evidence"`,
		`"small"`,
		`"medium"`,
		`"large"`,
		`(2, 3, 4)`,
		`"cpu_ms"`,
		`"peak_rss_bytes"`,
		`"failure_categories"`,
		`X-HotKey-Agent-Token`,
	} {
		if !strings.Contains(tool, fragment) {
			t.Errorf("Agent capacity tool lost %q", fragment)
		}
	}

	for _, composePath := range []string{"docker-compose.yml", "docker-compose-prod.yml"} {
		compose := readRepositoryFile(t, repository, composePath)
		block := dockerComposeServiceBlock(t, compose, "hotkey-agent")
		for _, fragment := range []string{
			"read_only: true",
			"cap_drop:",
			"- ALL",
			"no-new-privileges:true",
			"pids_limit: 128",
			`mem_limit: "512m"`,
			`cpus: "1.0"`,
			"/tmp:size=64m",
		} {
			if !strings.Contains(block, fragment) {
				t.Errorf("%s Agent isolation lost %q", composePath, fragment)
			}
		}
		if strings.Contains(block, "HOTKEY_DATABASE_URL") || strings.Contains(block, "HOTKEY_REDIS_URL") ||
			strings.Contains(block, "HOTKEY_MINIO") || strings.Contains(block, "ports:") {
			t.Errorf("%s Agent must not receive business stores or a published port", composePath)
		}
	}

	makefile := readRepositoryFile(t, repository, "backend/Makefile")
	if !strings.Contains(makefile, "agent-capacity-baseline:") ||
		!strings.Contains(makefile, "capacity_baseline.py") {
		t.Error("backend Makefile must expose the canonical Agent capacity calibration entry point")
	}

	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")
	row := markdownChecklistRow(t, plan, "CHK-003-G3-001")
	if !strings.HasPrefix(row, "- [x]") {
		t.Errorf("AC-003-001 capacity and isolation evidence exists but checklist is not complete: %s", row)
	}
	for _, evidence := range []string{
		"agent-capacity.v1",
		"make agent-capacity-baseline",
		"TestPythonAgentCapacityAndIsolationGateMatchesAC003001",
	} {
		if !strings.Contains(row, evidence) {
			t.Errorf("CHK-003-G3-001 does not cite %s: %s", evidence, row)
		}
	}
}
