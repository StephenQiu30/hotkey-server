---
layer: Operations
doc_no: "000"
audience: [Dev, QA, Ops]
feature_area: 项目运行与发布
purpose: 定义 HotKey Server 发布、运行、回滚和故障手册的归档规则
canonical_path: docs/operations/README.md
status: review
version: v1.5
owner: HotKey Server Team
inputs:
  - docs/README.md
outputs:
  - Operations 编写规范
triggers:
  - 新增发布、部署、回滚或运行流程
  - 运行方式或依赖恢复流程变化
downstream: []
---

# 运维文档规范

Operations 保存可重复执行的协作和运行流程，包括 Git/PR、发布、部署、备份、恢复、回滚、告警和故障处置。

## 必需内容

- 适用范围、前置权限和依赖
- 可复制命令与预期信号
- 失败判断、停止条件和回滚步骤
- 数据、密钥、日志和审计边界
- 验证方式和最后演练日期

## 收录边界

- 不放产品需求、架构设计或测试报告主体
- 不记录单次发布流水；只记录可重复流程
- 运行手册不得包含真实密钥、Token 或个人环境绝对路径
- 当前尚未设计部署拓扑，因此只建立规范入口，不虚构部署手册

## 当前手册

| 文档 | 说明 | 状态 | 最近演练 |
|---|---|---|
| [001-本地与GitHub CI质量门禁](001-本地与GitHub%20CI质量门禁.md) | `make ci` 的本地复现、GitHub Actions 流程及测试依赖边界 | accepted | 2026-07-17（PLAN-007 受控验收） |
| [PLAN007现有库受控升级](plan007-schema-upgrade.md) | PLAN-006 既有数据的备份、回填、legacy-zero、验证与回退 | accepted | 2026-07-17（[Acceptance-007](../acceptance/007-内容标准化去重与MinIO证据验收.md) accepted） |
| [PLAN008现有库受控升级](plan008-schema-upgrade.md) | 空 AI 历史库的备份、严格 preflight、add-only 升级、验证与历史 release 回退 | accepted | 2026-07-17（[Acceptance-008](../acceptance/008-AIProvider与Embedding基础验收.md) accepted） |
| [PLAN009 Task 1 Schema 升级](plan009-schema-upgrade.md) | relevance_review Profile 契约、固定 PLAN-008 历史库演练与回退 | review | 待 PLAN-009 Task 1 验收 |

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.2 | 2026-07-17 | 记录 PLAN-007 受控升级/回退与 CI 的最近演练；独立最终复核已通过。 |
| v1.3 | 2026-07-17 | 增加待实施的 PLAN-008 Schema 升级/回退手册，不表示 AI 功能已交付。 |
| v1.4 | 2026-07-17 | 记录 PLAN-008 的固定历史 worktree 升级/回退演练及 Acceptance-008 accepted 结论。 |
| v1.5 | 2026-07-17 | 新增 PLAN-009 Task 1 的 relevance_review Profile 升级/回退手册；验收前保持 review。 |
