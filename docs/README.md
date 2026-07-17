---
layer: Operations
doc_no: "000"
audience: [PM, Dev, QA, Ops]
feature_area: 项目文档
purpose: 定义 HotKey Server 正式文档的分类、归档和维护规则
canonical_path: docs/README.md
status: review
version: v1.7
owner: HotKey Server Team
inputs:
  - https://github.com/StephenQiu30/stephen-codex
outputs:
  - 文档目录约定
  - 完成任务归档约定
triggers:
  - 新增正式文档类型
  - 修改文档状态、关联或归档规则
downstream:
  - docs/design/README.md
  - docs/prd/README.md
  - docs/plans/README.md
  - docs/acceptance/README.md
  - docs/operations/README.md
---

# 项目文档规范

`docs/` 只保存会长期影响 HotKey Server 开发、验收、发布或维护决策的正式文档。需求、设计、计划、验收和运维材料必须进入对应目录，不能混放。

本文档只约定仓库中的文档如何分类、查找和归档，不参与服务启动或业务运行。

## 当前交付状态

目标设计不能代替当前实现状态。001–012 已完成并分别移入 `design/archive/`、`prd/archive/`、`plans/archive/` 和 `acceptance/archive/`；PLAN-013–017 仍保留在当前目录并处于 `backlog`，不能描述为已上线或已验收。本地启动、GitHub CI、发布和故障处置等可重复运行流程归入 [Operations](operations/README.md)。

## 文档层级

| 目录 | 回答的问题 | 内容 |
|---|---|---|
| [design](design/README.md) | 系统应当如何设计 | 架构、数据、接口、状态机、算法与技术取舍 |
| [prd](prd/README.md) | 任务必须交付什么 | 目标、范围、非目标、功能要求和首版验收门禁 |
| [plans](plans/README.md) | 任务具体如何执行 | 开工条件、文件清单、步骤、验证命令和提交边界 |
| [acceptance](acceptance/README.md) | 如何证明任务完成 | 红绿证据、测试记录、验收结论和残余风险 |
| [operations](operations/README.md) | 如何发布与运行 | Git/PR、GitHub CI、发布、部署、运行、回滚和故障手册 |
| [design/archive](design/archive/README.md) | 已完成设计放在哪里 | 已落地设计基线 |
| [prd/archive](prd/archive/README.md) | 已完成 PRD 放在哪里 | 001–012 的历史任务需求 |
| [plans/archive](plans/archive/README.md) | 已完成 Plan 放在哪里 | 001–012 的历史执行计划 |
| [acceptance/archive](acceptance/archive/README.md) | 已完成验收放在哪里 | 001–012 的长期验收证据 |

## 文档状态

正式文档保留简短 frontmatter，至少说明文档类型、编号、状态和用途。PRD/Plan 使用 `execution_status` 标记 `backlog`、`ready`、`in_progress` 或 `done`。通过验收并完成的内容移入所属目录的 `archive/`；未完成内容不提前归档。

## 关联规则

PRD、Plan 和 Acceptance 应在正文中保留对应关系和验收提交；移动到 archive 后同步更新路径。Operations 只保存可重复执行的运行、发布和回滚流程。

## 内容边界

- 正式文档只写稳定需求、决策、执行契约和可复用证据
- 临时 todo、工时、日报、会议流水、一次性排查和中间推演不得进入 `docs/`
- 未实现目标必须明确标记，不能描述成当前能力
- 数据库的唯一可执行事实源是完整 `db/schema.sql`
- 公共接口的唯一可执行契约是生成并校验后的 OpenAPI
- 单次实施进度应保留在 ticket、Workpad 或 PR，不回写正式 Plan 正文

## 更新顺序

涉及需求或架构变化时，按以下顺序更新：

1. Design（若设计发生变化）
2. PRD 与 Plan
3. 代码、完整 Schema、记录模型、OpenAPI 和测试
4. Acceptance
5. 必要的 Operations
