## Why

知识中台当前仅有 `HotKey → Obsidian` 的单向导出链路，人工在 Obsidian 中对专题标签、分析结论和素材状态的结构化编辑无法回流到 HotKey，导致机器事实与人工知识长期漂移。需要建立白名单驱动的结构化回写链路，配合冲突检测和审计日志，形成知识中台的可控闭环。本变更对应 `docs/plans/025-双向回写与治理计划.md`。

## What Changes

- 新增 `knowledge_writeback_logs` 审计表（db/schema.sql）
- 实现 YAML frontmatter 白名单字段解析器（internal/obsidian/writeback_parser.go）
- 实现回写字段校验器（internal/knowledge/validator.go），拒绝非白名单字段
- 实现修订冲突检测（internal/knowledge/conflict.go）
- 实现 sidecar 回写应用服务（internal/knowledge/service.go）
- 实现 sidecar repository（event_annotation_repo、topic_annotation_repo、theme_membership_repo）
- 实现批量回写 job（internal/jobs/apply_knowledge_writeback.go）
- 实现 roundtrip 集成测试（tests/integration/knowledge_writeback_roundtrip_test.go）
- 修改 scripts/validate-repository.sh 追加 roundtrip 回归

## Capabilities

### New Capabilities
- `writeback-white-list`: 定义并校验可回写的结构化字段白名单（manual_tags, analyst_conclusion, theme_ref, material_status）
- `writeback-conflict-detection`: 基于 revision digest 的写入冲突检测
- `writeback-audit-log`: 所有回写操作的审计记录（含状态、冲突原因、源路径）
- `writeback-roundtrip`: 导出→人工修改→回写→再导出 的全链路一致性验证

### Modified Capabilities
- `daily-digest`: 修改其验证回归流程，在知识快照回归后追加 roundtrip 回归

## Impact

- 新增 `internal/knowledge/` 包（validator、conflict、service）
- 新增 `internal/database/knowledge_writeback_repo.go` 审计 repo
- 新增 `internal/database/event_annotation_repo.go`、`topic_annotation_repo.go`、`theme_membership_repo.go`
- 新增 `internal/obsidian/writeback_parser.go`
- 新增 `internal/jobs/apply_knowledge_writeback.go`
- 新增 `tests/integration/knowledge_writeback_roundtrip_test.go`
- 修改 `db/schema.sql`（新增 knowledge_writeback_logs 表）
- 修改 `scripts/validate-repository.sh`
