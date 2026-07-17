---
layer: PRD
doc_no: "000"
audience: [PM, Dev, QA, Ops]
feature_area: AI热点事件监控平台
purpose: 管理从权威设计拆分出的后端执行任务需求
canonical_path: docs/prd/README.md
status: review
version: v3.0
owner: HotKey Server Team
inputs:
  - docs/README.md
  - docs/design/README.md
outputs:
  - PRD 任务索引
  - PRD 依赖图
triggers:
  - 新增、拆分、合并或替代任务
  - Design 变化影响任务边界
downstream:
  - docs/plans/README.md
  - docs/acceptance/README.md
---

# 执行任务 PRD 索引

本目录把 docs/design/001–015 的目标设计转换为可排期、可实现、可测试、可验收的后端任务。设计文档回答“系统应当是什么”，PRD 回答“下一项实现交付什么、依赖什么、如何证明完成”。

## 使用规则

- 设计文档、AGENTS.md 和完整 db/schema.sql 仍是架构与数据库事实源，PRD 不复制或修改设计结论。
- `status` 表示文档成熟度，只使用 draft、review、accepted、archived。
- `execution_status` 表示实现进度，只使用 backlog、ready、in_progress、blocked、done、superseded。
- PRD 只有 `status: accepted` 且 `execution_status: ready` 才能进入开工候选；对应 Plan 还必须满足 `status: accepted`、`review_status: approved` 与 `execution_status: ready`，任一条件不满足均不得写代码。
- 实施中发现设计缺口时先更新 docs/design，再更新受影响 PRD，不得在代码中隐式决定新架构。
- PRD 只记录稳定范围、依赖、交付物和验收条件，不记录日报、调试流水或人员工时。
- 每个任务必须同步其涉及的代码、完整 db/schema.sql、数据库记录模型、OpenAPI、测试和架构校验；不涉及的事实源应在 PR 说明中明确标记。
- 本目录只覆盖 hotkey-server，不包含 Web 或 Miniapp 页面实现。

## 执行状态

| 状态 | 含义 |
|---|---|
| backlog | 已拆分，等待文档接受或前置任务 |
| ready | 前置任务完成且允许开工 |
| in_progress | 正在实施，只允许一个主要负责人推进 |
| blocked | 前置依赖或外部条件阻塞，必须写明阻塞原因 |
| done | 交付物、测试、文档和验收证据全部完成 |
| superseded | 已被新 PRD 替代，不得继续实施 |

## 阶段与依赖

| PRD | 任务 | 阶段 | 优先级 | 依赖 | 文档 | 执行 |
|---|---|---|---|---|---|---|
| [001](001-模块化单体启动与工程门禁.md) | 模块化单体、Schema基线与工程门禁 | F0 | P0 | 无 | archived | done |
| [002](002-单一Schema与数据库平台.md) | 数据库运行时、事务与兼容性平台 | F0 | P0 | 001 | archived | done |
| [003](003-HTTP契约安全与可观测基础.md) | HTTP 契约、安全与可观测基础 | F0 | P0 | 001, 002 | archived | done |
| [004](004-身份认证会话与权限.md) | 身份、认证、会话与权限 | F0 | P0 | 002, 003 | archived | done |
| [005](005-监控主题规则与来源配置.md) | 监控主题、规则与来源配置 | P0 | P0 | 002, 003, 004 | archived | done |
| [018](018-任务执行与计划归档治理.md) | 任务执行与计划归档治理 | Governance | P0 | 005 | accepted | ready |
| [006](006-查询规划与RSS-HN采集.md) | 查询规划与 RSS/HN 采集 | P0 | P0 | 005 | archived | done |
| [007](007-内容标准化去重与MinIO证据.md) | 内容标准化、去重与 MinIO 证据 | P0 | P0 | 002, 006 | archived | done |
| [008](008-AIProvider与Embedding基础.md) | AI Provider 与 Embedding 基础 | P0 | P0 | 002, 007 | archived | done |
| [009](009-多语言相关性匹配与反馈.md) | 多语言相关性匹配与反馈 | P0 | P0 | 005, 007, 008 | archived | done |
| [010](010-事件聚类生命周期与人工治理.md) | 事件聚类、生命周期与人工治理 | P0 | P0 | 009 | archived | done |
| [011](011-热度趋势与监控排序.md) | 热度、趋势与监控排序 | P0 | P0 | 010 | archived | done |
| [012](012-证据化事件摘要实体与主张.md) | 证据化事件摘要、实体与主张 | P0 | P0 | 008, 010 | review | backlog |
| [013](013-Cron与River主链路编排.md) | Cron 与 River 主链路编排 | P0 | P0 | 006–012 | review | backlog |
| [014](014-Obsidian知识提案修订与对账.md) | Obsidian 知识提案、修订与对账 | P1 | P1 | 010, 012, 013 | review | backlog |
| [015](015-日报周报与发布快照.md) | 日报、周报与发布快照 | P1 | P1 | 011, 012, 013 | review | backlog |
| [016](016-邮件与RSS-Atom订阅交付.md) | 邮件与 RSS/Atom 订阅交付 | P1 | P1 | 014, 015 | review | backlog |
| [017](017-运行治理容量与端到端验收.md) | 运行治理、容量与端到端验收 | Closure | P0 | 001–016 | review | backlog |

## 主链路

    001 → 002 → 003 → 004 → 005 → 006 → 007 → 008
                                      005 → 018（非阻塞治理支线）
                                      007 + 008 → 009 → 010 → 011
                                                   008 + 010 → 012
                                      006–012 → 013
                                      010 + 012 + 013 → 014
                                      011 + 012 + 013 → 015
                                      014 + 015 → 016
                                      001–016 → 017

PLAN-008–011 已归档为 `archived/done`。PLAN-011 已完成同步热度重算、版本化快照与排序验收；实际采集到长期线上质量的编排由 PLAN-013/017 验收。PLAN-012–017 尚未通过各自的 Plan Review 和依赖准入，保持 `backlog`，不得描述为正在实施。

## 每个 PRD 的完成要求

一个任务只有同时满足以下条件才能标记 done：

1. 范围内的行为与非目标没有被扩大。
2. 领域、应用、基础设施和 HTTP 边界符合 AGENTS.md。
3. 涉及数据库时，完整 db/schema.sql、记录模型和 Repository 测试一致。
4. 涉及公共接口时，OpenAPI 与统一 Result 契约一致。
5. 单元、集成、故障和幂等测试覆盖该任务风险。
6. make lint、make test、make build、make validate 通过；涉及契约时额外通过 OpenAPI 校验。
7. 验收证据可在 PR 或长期验收文档中复核。
