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
		"backend/test/tools/joint-recovery-drill/main.go": {
			`Version: "hotkey-joint-recovery-v1"`,
			`"postgres_facts"`,
			`"minio_evidence"`,
			`"vault_all_files"`,
			`"vault_manual_regions"`,
			`"river_jobs_attempts"`,
			`exec.CommandContext(ctx, "pg_dump"`,
			`exec.CommandContext(ctx, "pg_restore"`,
			`ProductionEgressDisabled`,
			`os.O_WRONLY|os.O_CREATE|os.O_EXCL`,
		},
		"docs/acceptance/evidence/001/contents-keyset-capacity-macos-arm64-3d9acccb.json": {
			`"version": "hotkey-capacity-baseline-v1"`,
			`"status": "measured"`,
			`"git_revision": "3d9acccbc136195ee1c26fa7d9cec69cef2d1740"`,
			`"fixture_rows": 100000`,
			`"concurrency": 20`,
			`"samples": 1000`,
			`"errors": 0`,
		},
		"docs/acceptance/evidence/001/joint-recovery-macos-arm64-3d9acccb.json": {
			`"version": "hotkey-joint-recovery-v1"`,
			`"status": "reconciled"`,
			`"git_revision": "3d9acccbc136195ee1c26fa7d9cec69cef2d1740"`,
			`"rpo_millis": 165`,
			`"rto_millis": 762`,
			`"expected_versioned_count": 2`,
			`"actual_versioned_count": 2`,
			`"differences": []`,
		},
		"backend/Makefile": {
			"joint-recovery-acceptance:",
		},
		".github/workflows/ci.yml": {
			"Joint PostgreSQL MinIO Vault and River recovery acceptance",
			"make joint-recovery-acceptance",
		},
		"docs/operations/004-可观测性SLO与事件响应.md": {
			"make capacity-fixture",
			"make capacity-baseline",
			"不代表普通 API、热点列表或全文检索已达标",
		},
		"docs/operations/002-备份恢复与重建.md": {
			"hotkey-recovery-manifest-v1",
			"make recovery-evidence",
			"make joint-recovery-acceptance",
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
	if !strings.Contains(plan, "- [x] `CHK-001-G3-002`") ||
		!strings.Contains(plan, "contents-keyset-capacity-macos-arm64-3d9acccb.json") ||
		!strings.Contains(plan, "joint-recovery-macos-arm64-3d9acccb.json") {
		t.Error("capacity/recovery checklist must cite the measured baseline and zero-difference joint restore")
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
		"docs/acceptance/evidence/002/collection-capacity-macos-arm64-6f5a3e03.json": {
			`"version": "hotkey-collection-capacity-v1"`,
			`"status": "measured"`,
			`"git_revision": "6f5a3e0367951a28a35a5c286e680639ac0971cc"`,
			`"active_monitors": 50`,
			`"enabled_source_connections": 100`,
			`"candidate_items": 50000`,
			`"collection_jobs": 20`,
			`"external_connector_network_latency"`,
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
	if !strings.Contains(plan, "docs/acceptance/evidence/002/collection-capacity-macos-arm64-6f5a3e03.json") {
		t.Error("collection capacity candidate evidence is not traceable from plan 002")
	}
	if !strings.Contains(plan, "- [ ] `CHK-002-G5-001`") {
		t.Error("collection capacity checklist was completed without approved real-source and fixed-environment evidence")
	}
}

func TestCodexCapacityEvidenceKeepsSyntheticAndLiveApprovalSeparate(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	contracts := map[string][]string{
		"backend/test/tools/codex-capacity-baseline/main.go": {
			`Version: "hotkey-codex-capacity-v1"`,
			`Approval: "required"`,
			`PercentileAlgorithm: "nearest-rank-ceiling"`,
			`[]int{2, 3, 4}`,
			`[]string{"content.relevance.v1", "event.brief.v1"}`,
			"ProcessCPUTimeMicros",
			"PeakRSSBytes",
			"os.O_WRONLY|os.O_CREATE|os.O_EXCL",
		},
		"backend/Makefile": {"codex-capacity-baseline:"},
		"docs/operations/004-可观测性SLO与事件响应.md": {
			"make codex-capacity-baseline",
			"Fake 模式只验证进程、测量和报告契约",
			"Live 模式也只产生 `candidate` 报告，不自动批准并发",
		},
	}
	for relative, required := range contracts {
		payload := readRepositoryFile(t, repository, relative)
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s is missing Codex capacity evidence contract %q", relative, fragment)
			}
		}
	}
	plan := readRepositoryFile(t, repository, "docs/plans/003-智能研判事件热度与人工治理计划.md")
	if !strings.Contains(plan, "- [x] `CHK-003-G3-001`") || !strings.Contains(plan, "`agent-capacity.v1`") {
		t.Error("Python Agent capacity checklist must cite its own approved Agent baseline")
	}
	operations := readRepositoryFile(t, repository, "docs/operations/004-可观测性SLO与事件响应.md")
	if !strings.Contains(operations, "Codex Live 容量报告仍是遗留迁移候选") ||
		!strings.Contains(operations, "不能替代 `agent-capacity.v1`") {
		t.Error("Codex candidate capacity must remain separate from the approved Python Agent baseline")
	}
}

func TestM4ProjectionRecoveryKeepsFactsHumanRegionsAndAgentBoundary(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	contracts := map[string][]string{
		"backend/db/schema.sql": {
			"projection_recovery_runs",
			"projection_recovery_runs_append_only",
			"reject_projection_recovery_run_mutation",
		},
		"backend/internal/modules/operations/application/projection_recovery.go": {
			"ProjectionRecoveryFactSnapshotDTO",
			"VaultManualRegionFingerprintSHA256",
			"ExpectedStartedClaimCount",
			"ExpectedUnknownAttemptCount",
			"ErrProjectionRecoveryIntegrity",
		},
		"backend/internal/modules/operations/infrastructure/postgres/projection_recovery_repository.go": {
			"notification_outbox_events",
			"notification_read_receipts",
			"projection_recovery_runs",
			"dispatch_started_at IS NULL",
			"started_delivery_claim_requires_provider_reconciliation",
			"queue.KindProjectKnowledge",
			"queue.KindGenerateSourceDocument",
		},
		"backend/internal/modules/knowledge/application/recovery.go": {
			"HumanRegionSHA256",
			"func (service *VaultRecoveryService) Inspect",
			"Inspect verifies the same protected source chain as Recover but never",
		},
		"backend/internal/bootstrap/projection_recovery_command.go": {
			"--dry-run",
			"confirm-isolated",
			"production-egress-disabled",
			"non-production configuration with SMTP disabled",
		},
		"docs/operations/002-备份恢复与重建.md": {
			"maintenance recover-projections --dry-run",
			"不可变恢复运行记录",
			"started_delivery_claim_requires_provider_reconciliation",
			"Python Agent 不参与恢复编排",
		},
	}
	for relative, required := range contracts {
		payload := readRepositoryFile(t, repository, relative)
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s is missing M4 projection recovery contract %q", relative, fragment)
			}
		}
	}
	plan := readRepositoryFile(t, repository, "docs/plans/004-通知报告知识投影与检索计划.md")
	if !strings.Contains(plan, "- [ ] `CHK-004-G5-001`") {
		t.Error("M4 recovery checklist was completed before an isolated end-to-end recovery run")
	}
}
