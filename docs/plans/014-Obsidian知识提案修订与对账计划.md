---
layer: Plan
doc_no: "014"
audience: [Dev, QA, Ops]
feature_area: 知识库治理
purpose: 实施 Obsidian 提案、审核、修订、原子写入与跨存储对账
canonical_path: docs/plans/014-Obsidian知识提案修订与对账计划.md
status: review
execution_status: backlog
review_status: pending
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/014-Obsidian知识提案修订与对账.md
  - docs/plans/010-事件聚类生命周期与人工治理计划.md
  - docs/plans/012-证据化事件摘要实体与主张计划.md
  - docs/plans/013-Cron与River主链路编排计划.md
outputs:
  - knowledge 模块
  - Vault 原子投影与对账
triggers:
  - PRD-014 accepted 且 ready
downstream:
  - docs/acceptance/014-Obsidian知识提案修订与对账验收.md
depends_on: [PLAN-010, PLAN-012, PLAN-013]
---

# Obsidian 知识提案、修订与对账计划

## 计划目标

把 Event 和 Topic 安全投影到 Vault，通过提案、审核、哈希、修订和冲突机制保护人工内容。

## 开工条件

- 当前 Plan 的 status 为 accepted、review_status 为 approved、execution_status 为 ready
- 对应 PRD 的 status 为 accepted，execution_status 为 ready
- frontmatter 中 depends_on 列出的 Plan 全部为 done
- main 已同步，工作区只包含当前任务相关文件

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | internal/modules/knowledge/domain/*.go | Document、Proposal、Revision 与状态 |
| 创建 | internal/modules/knowledge/application/proposal.go | 提案生成与审核 |
| 创建 | internal/modules/knowledge/application/projector.go | Event/Topic 投影 |
| 创建 | internal/modules/knowledge/application/reconciliation.go | 三方对账 |
| 创建 | internal/modules/knowledge/infrastructure/postgres/*.go | 知识持久化 |
| 创建 | internal/modules/knowledge/infrastructure/vault/*.go | 路径锁与原子写入 |
| 创建 | internal/modules/knowledge/templates/*.md | Event、Topic Frontmatter 模板 |
| 创建 | internal/modules/knowledge/transport/http/*.go | 提案、审核、修订与对账 API |
| 创建 | internal/modules/knowledge/infrastructure/jobs/*.go | proposal、project、reconcile Job |
| 修改 | db/schema.sql | knowledge、revision、vault run 表 |
| 创建 | internal/modules/knowledge/**/*_test.go | 路径、冲突、崩溃与恢复测试 |

## 执行步骤

1. 先写路径逃逸、人工编辑、并发写和崩溃残留红灯测试。
2. 同步知识文档、提案、修订和运行 Schema。
3. 实现稳定路径、Frontmatter 和自动/人工区域。
4. 实现提案审核、base_hash 校验和冲突状态。
5. 实现临时文件、flush、原子重命名和 MinIO 快照。
6. 接入 River Job、三方对账和管理员 API。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test ./internal/modules/knowledge/... -count=1 | 路径与冲突测试失败 |
| 绿灯 | go test ./internal/modules/knowledge/... -count=1 | 全部通过 |
| 故障 | go test -tags=integration ./internal/modules/knowledge/... -run VaultFailure -count=1 | 崩溃与恢复通过 |
| 对账 | go test -tags=integration ./internal/modules/knowledge/... -run Reconciliation -count=1 | DB/MinIO/Vault 对账通过 |
| 全量 | make ci | 全部通过 |

## 验收清单

- 事件与主题路径和 Frontmatter 稳定
- 人工修改后旧提案进入 conflict
- 自动更新不覆盖人工区域
- 每次修改有可恢复修订和 MinIO 快照
- 路径遍历与符号链接逃逸被拒绝
- 无 Git 环境下知识流程完整运行

## 提交边界

- test: 定义 Vault 安全与冲突门禁
- impl: 实现知识提案、修订和原子写入
- feat: 接入知识投影与对账 Job


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现
