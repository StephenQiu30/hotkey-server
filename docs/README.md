---
layer: Operations
doc_no: "000"
audience: [PM, Dev, QA, Ops]
feature_area: 文档治理
purpose: 定义 HotKey Server 正式文档的分类、元数据、关联和维护规则
canonical_path: docs/README.md
status: review
version: v1.1
owner: HotKey Server Team
inputs:
  - https://github.com/StephenQiu30/stephen-codex
outputs:
  - 五层文档规范
  - 正式文档 frontmatter 规范
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

本规范基于 [stephen-codex](https://github.com/StephenQiu30/stephen-codex/tree/66f1fadc4999c7bcd646e25dddff5aad67215007/docs) 文档骨架，并按 HotKey Server 的模块化单体、后端边界、完整 Schema 和 OpenAPI 事实源进行了项目化调整。

## 当前交付状态

目标设计不能代替当前实现状态。任务进度只在 [PRD 索引](prd/README.md) 和 [Plan 索引](plans/README.md) 维护：001–005 已完成并由 [Acceptance](acceptance/README.md) 保存长期证据；治理任务 018 已审核为非阻塞 `ready` 支线，产品任务 006–017 仍处于 `review` / `backlog`。本地启动、GitHub CI、发布和故障处置等可重复运行流程归入 [Operations](operations/README.md)。

## 文档层级

| 目录 | 回答的问题 | 内容 |
|---|---|---|
| [design](design/README.md) | 系统应当如何设计 | 架构、数据、接口、状态机、算法与技术取舍 |
| [prd](prd/README.md) | 任务必须交付什么 | 目标、范围、非目标、功能要求和首版验收门禁 |
| [plans](plans/README.md) | 任务具体如何执行 | 开工条件、文件清单、步骤、验证命令和提交边界 |
| [acceptance](acceptance/README.md) | 如何证明任务完成 | 红绿证据、测试记录、验收结论和残余风险 |
| [operations](operations/README.md) | 如何发布与运行 | Git/PR、GitHub CI、发布、部署、运行、回滚和故障手册 |

## 必需 frontmatter

每份正式正文文档必须包含：

1. `layer`：Design、PRD、Plan、Acceptance 或 Operations
2. `doc_no`：三位字符串编号
3. `audience`：PM、Dev、QA、Ops 中的适用读者
4. `feature_area`：所属功能域
5. `purpose`：一句话说明文档目的
6. `canonical_path`：仓库内唯一标准路径
7. `status`：draft、review、accepted、archived
8. `version`：文档版本
9. `owner`：维护责任方
10. `inputs`：输入或前置正式文档
11. `outputs`：本文档形成的长期决策或交付物
12. `triggers`：何时必须阅读或更新
13. `downstream`：受本文档约束的下游文档

PRD 和 Plan 额外使用 `execution_status`：backlog、ready、in_progress、blocked、done、superseded。Plan 还必须使用 `review_status`：pending、in_review、approved、changes_requested。文档成熟度、计划审核和代码执行状态不能共用一个字段。

## 关联规则

1. PRD 必须关联来源 Design、对应 Plan 和目标 Acceptance。
2. Plan 必须关联一个 PRD、相关 Design、前置 Plan 和目标 Acceptance。
3. Design 必须列出受影响 PRD、Plan 或 Acceptance。
4. Acceptance 必须关联被验收的 PRD、Plan、Design 和准确提交。
5. Operations 必须关联适用的发布、部署、运行或回滚对象。

## 内容边界

- 正式文档只写稳定需求、决策、执行契约和可复用证据
- 临时 todo、工时、日报、会议流水、一次性排查和中间推演不得进入 `docs/`
- 未实现目标必须明确标记，不能描述成当前能力
- 数据库的唯一可执行事实源是完整 `db/schema.sql`
- 公共接口的唯一可执行契约是生成并校验后的 OpenAPI
- 单次实施进度应保留在 ticket、Workpad 或 PR，不回写正式 Plan 正文

## 更新顺序

涉及需求或架构变化时，按以下顺序更新：

1. Design
2. PRD
3. Plan
4. 独立 Plan Review 与验收标准复核
5. 代码、完整 Schema、记录模型、OpenAPI 和测试
6. Acceptance
7. 必要的 Operations
- 旧编号文档不作为历史兼容方案保留；替代设计确认后直接从仓库清除。
