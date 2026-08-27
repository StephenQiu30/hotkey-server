package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestS03CapacityAndRecoveryEvidenceStayMeasuredAndReproducible(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	contracts := map[string][]string{
		"backend/test/tools/capacity-baseline/main.go": {
			`Version: "hotkey-capacity-baseline-v1"`,
			`PercentileAlgorithm: "nearest-rank-ceiling"`,
			`DurationMicros`,
			`os.O_WRONLY|os.O_CREATE|os.O_EXCL`,
			`HOTKEY_CAPACITY_HARDWARE`,
			`HOTKEY_CAPACITY_GIT_REVISION`,
		},
		"backend/test/tools/recovery-evidence/main.go": {
			`"postgres_facts": false`,
			`"minio_evidence": false`,
			`"vault_all_files": false`,
			`"vault_manual_regions": false`,
			`"river_jobs_attempts": false`,
			`CandidateRPOMet`,
			`CandidateRTOMet`,
			`os.O_WRONLY|os.O_CREATE|os.O_EXCL`,
		},
		"docs/operations/004-可观测性SLO与事件响应.md": {
			"make capacity-fixture",
			"make capacity-baseline",
			"不代表普通 API、热点列表或全文检索已达标",
		},
		"docs/operations/002-备份恢复与重建.md": {
			"hotkey-recovery-manifest-v1",
			"make recovery-evidence",
			"incident_cutoff_at - recovery_point_at",
			"services_readable_at - drill_started_at",
			"不替代本手册的真实备份、恢复和对账命令",
		},
	}
	for relative, required := range contracts {
		payload := readRepositoryFile(t, repository, relative)
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s is missing capacity/recovery evidence contract %q", relative, fragment)
			}
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/001-HotKey产品需求分析与总体架构计划.md")
	if !strings.Contains(plan, "- [ ] `CHK-001-G3-002`") {
		t.Error("capacity/recovery checklist was completed without a real isolated measurement and joint restore")
	}
}

func TestS04CollectionCapacityEvidenceCoversTheApprovedWorkload(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	contracts := map[string][]string{
		"backend/test/tools/generate-collection-capacity-fixture.sh": {
			"HOTKEY_COLLECTION_CAPACITY_MONITORS:-50",
			"HOTKEY_COLLECTION_CAPACITY_SOURCES:-100",
			"HOTKEY_COLLECTION_CAPACITY_CANDIDATES:-50000",
			"HOTKEY_COLLECTION_CAPACITY_JOBS:-20",
			"FOR UPDATE SKIP LOCKED",
		},
		"backend/test/tools/collection-capacity-baseline/main.go": {
			`Version: "hotkey-collection-capacity-v1"`,
			`PercentileAlgorithm: "nearest-rank-ceiling"`,
			`Route: route, Stack: "httptest_http+gin+authz+application+postgres+dto+json"`,
			"ThroughputRowsPerSecond",
			"runtimeDB.Pool.Config().MaxConns",
			"os.O_WRONLY|os.O_CREATE|os.O_EXCL",
			"HOTKEY_COLLECTION_CAPACITY_GIT_REVISION",
		},
		"backend/Makefile": {
			"collection-capacity-fixture:",
			"collection-capacity-baseline:",
		},
		"docs/operations/004-可观测性SLO与事件响应.md": {
			"make collection-capacity-fixture",
			"make collection-capacity-baseline",
			"真实来源授权冒烟、冷缓存和生产同构硬件结果仍须独立执行并审批",
		},
	}
	for relative, required := range contracts {
		payload := readRepositoryFile(t, repository, relative)
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s is missing collection capacity evidence contract %q", relative, fragment)
			}
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/002-监控来源采集与证据链计划.md")
	if !strings.Contains(plan, "- [ ] `CHK-002-G5-001`") {
		t.Error("collection capacity checklist was completed without approved real-source and fixed-environment evidence")
	}
}
