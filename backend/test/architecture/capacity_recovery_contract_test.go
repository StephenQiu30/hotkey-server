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
			"当前本机与固定 CI 环境就是本轮容量验收基线",
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
	if !strings.Contains(plan, "- [x] `CHK-002-G5-001`") {
		t.Error("collection capacity checklist must accept the approved local and fixed-CI evidence")
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
		"backend/test/_suite/internal/modules/ingestion/infrastructure/postgres/m4_projection_recovery_drill_integration_test.go": {
			`hotkey-m4-projection-recovery-drill-v1`,
			`--exclude-table-data=public.document_version_search_indexes`,
			`TestM4ProjectionRecoveryDrillRestoresIndependentCopyToZeroDifference`,
			`queue.KindProjectKnowledge`,
			`queue.KindGenerateSourceDocument`,
			`manual_provider_reconciliation_required_before_any_replay`,
			`os.O_WRONLY|os.O_CREATE|os.O_EXCL`,
			`0o600`,
		},
		"backend/Makefile": {
			"m4-fault-recovery-acceptance: test-env minio-test-env",
			"TestM4ProjectionRecoveryDrillRestoresIndependentCopyToZeroDifference",
		},
		".github/workflows/ci.yml": {
			"Upload sanitized M4 projection recovery evidence",
			"HOTKEY_M4_PROJECTION_RECOVERY_PRODUCTION_EGRESS_DISABLED",
		},
		"docs/acceptance/evidence/004/m4-projection-recovery-github-ubuntu-62af91fd.json": {
			`"version": "hotkey-m4-projection-recovery-drill-v1"`,
			`"git_revision": "62af91fd34cce491cdd2306cb7f60ba1bd6b83e6"`,
			`"postgresql": true`,
			`"provider_calls": 0`,
			`"blind_replays": 0`,
			`"revocation_visible_micros": 8832`,
			`"differences": []`,
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
			"HOTKEY_M4_PROJECTION_RECOVERY_OUTPUT",
			"不把 `document_version_search_indexes` 的可丢弃数据写入恢复包",
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
	if !strings.Contains(plan, "- [x] `CHK-004-G5-001`") ||
		!strings.Contains(plan, "m4-projection-recovery-github-ubuntu-62af91fd.json") {
		t.Error("M4 recovery checklist must cite the verified independent zero-difference recovery run")
	}
}

func TestM4BusinessFlowCapacityUsesTheFreshStackAndKeepsApprovalExplicit(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	contracts := map[string][]string{
		"frontend/test/browser/business-flow-capacity.mjs": {
			`hotkey-m4-business-flow-capacity-v1`,
			`daily_report_to_notification_to_vault_to_postgresql_search`,
			`nearest-rank-ceiling`,
			`X-Request-ID`,
			`runtime_observation`,
			`projection_backlog`,
			`bounded_resource_samples`,
			`productionEgressDisabled !== true`,
			`flag: "wx", mode: 0o600`,
		},
		"frontend/test/browser/business-flow-runtime-observation.mjs": {
			`docker`,
			`stats`,
			`hotkey_runtime_metrics_collection_success`,
			`deliver_email`,
			`build_report`,
			`project_knowledge`,
			`generate_source_document`,
			`isolated container prefix`,
		},
		"frontend/test/browser/business-flow-capacity.test.mjs": {
			`measures report, notification, Vault and PostgreSQL search through the real HTTP contract`,
			`writes one exclusive sanitized artifact`,
			`fails closed when required resource or projection-backlog observations are unavailable`,
		},
		"frontend/test/browser/business-flow-runtime-observation.test.mjs": {
			`collects bounded service resources and four projection backlogs`,
			`fails closed for formal containers, incomplete stats, failed metric collection or malformed numbers`,
		},
		".github/workflows/ci.yml": {
			`Measure fixed-environment M4 business-flow capacity`,
			`node frontend/test/browser/measure-business-flow-capacity.mjs`,
			`HOTKEY_M4_CAPACITY_FILESYSTEM`,
			`HOTKEY_M4_CAPACITY_INTER_FLOW_INTERVAL_MILLIS: "31000"`,
			`/tmp/hotkey-m4-capacity.json`,
		},
		"docs/operations/004-可观测性SLO与事件响应.md": {
			`hotkey-m4-business-flow-capacity-v1`,
			`HOTKEY_M4_CAPACITY_FILESYSTEM`,
			`resource`,
			`projection_backlog`,
			`HOTKEY_M4_CAPACITY_PRODUCTION_EGRESS_DISABLED=true`,
		},
		"docs/acceptance/evidence/004/m4-business-flow-capacity-github-ubuntu-a577fa1f.json": {
			`"version": "hotkey-m4-business-flow-capacity-v1"`,
			`"status": "measured"`,
			`"approval": "required"`,
			`"git_revision": "a577fa1f7de64003c3057e9685fc282e03a7057d"`,
			`"observed": 127`,
			`"unique": 127`,
			`"sentinel_leaks": 0`,
			`"errors": 0`,
		},
		"docs/acceptance/evidence/004/m4-sentinel-scan-github-ubuntu-a577fa1f.json": {
			`"version": "hotkey-sentinel-scan-v1"`,
			`"sentinels": 5`,
			`"leaks": 0`,
		},
		"docs/acceptance/evidence/004/m4-secret-surface-scan-github-ubuntu-a577fa1f.json": {
			`"version": "hotkey-secret-surface-scan-v2"`,
			`"files_scanned": 607`,
			`"bytes_scanned": 9558825`,
			`"leaks": []`,
		},
	}
	for relative, required := range contracts {
		payload := readRepositoryFile(t, repository, relative)
		for _, fragment := range required {
			if !strings.Contains(payload, fragment) {
				t.Errorf("%s is missing M4 business-flow capacity contract %q", relative, fragment)
			}
		}
	}

	plan := readRepositoryFile(t, repository, "docs/plans/004-通知报告知识投影与检索计划.md")
	if !strings.Contains(plan, "hotkey-m4-business-flow-capacity-v1") ||
		!strings.Contains(plan, "- [x] `CHK-004-G5-002`") {
		t.Error("M4 capacity evidence must close the approved fixed-environment baseline")
	}
}
